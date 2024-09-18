package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
)

const sqlInsertChannelLog = `
INSERT INTO channels_channellog( uuid,  log_type,  channel_id,  http_logs,  errors,  is_error,  created_on,  elapsed_ms)
                         VALUES(:uuid, :log_type, :channel_id, :http_logs, :errors, :is_error, :created_on, :elapsed_ms)`

// channel log to be inserted into the database
type dbChannelLog struct {
	UUID      clogs.LogUUID     `db:"uuid"`
	Type      clogs.LogType     `db:"log_type"`
	ChannelID courier.ChannelID `db:"channel_id"`
	HTTPLogs  json.RawMessage   `db:"http_logs"`
	Errors    json.RawMessage   `db:"errors"`
	IsError   bool              `db:"is_error"`
	CreatedOn time.Time         `db:"created_on"`
	ElapsedMS int               `db:"elapsed_ms"`
}

// queues the passed in channel log to a writer
func queueChannelLog(b *backend, clog *courier.ChannelLog) {
	log := slog.With("log_uuid", clog.UUID, "log_type", clog.Type, "channel_uuid", clog.Channel().UUID())
	dbChan := clog.Channel().(*Channel)
	isError := clog.IsError()

	// depending on the channel log policy, we might be able to discard this log
	if dbChan.LogPolicy == LogPolicyNone || (dbChan.LogPolicy == LogPolicyErrors && !isError) {
		return
	}

	if b.dyLogWriter.Queue(clog.Log) <= 0 {
		log.With("storage", "dynamo").Error("channel log writer buffer full")
	}

	// if log is not attached to a call or message, need to write it to the database so that it is retrievable
	if !clog.Attached() {
		v := &dbChannelLog{
			UUID:      clog.UUID,
			Type:      clog.Type,
			ChannelID: dbChan.ID(),
			HTTPLogs:  jsonx.MustMarshal(clog.HttpLogs),
			Errors:    jsonx.MustMarshal(clog.Errors),
			IsError:   isError,
			CreatedOn: clog.CreatedOn,
			ElapsedMS: int(clog.Elapsed / time.Millisecond),
		}
		if b.dbLogWriter.Queue(v) <= 0 {
			log.With("storage", "db").Error("channel log writer buffer full")
		}
	}

	log.Debug("channel log queued")
}

type DBLogWriter struct {
	*syncx.Batcher[*dbChannelLog]
}

func NewDBLogWriter(db *sqlx.DB, wg *sync.WaitGroup) *DBLogWriter {
	return &DBLogWriter{
		Batcher: syncx.NewBatcher(func(batch []*dbChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeDBChannelLogs(ctx, db, batch)
		}, 1000, time.Millisecond*500, 1000, wg),
	}
}

func writeDBChannelLogs(ctx context.Context, db *sqlx.DB, batch []*dbChannelLog) {
	err := dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, batch)

	// if we received an error, try again one at a time (in case it is one value hanging us up)
	if err != nil {
		for _, v := range batch {
			err = dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, []*dbChannelLog{v})
			if err != nil {
				log := slog.With("comp", "db log writer", "log_uuid", v.UUID)

				if qerr := dbutil.AsQueryError(err); qerr != nil {
					query, params := qerr.Query()
					log = log.With("sql", query, "sql_params", params)
				}

				log.Error("error writing channel log", "error", err)
			}
		}
	}
}

type DynamoLogWriter struct {
	*syncx.Batcher[*clogs.Log]
}

func NewDynamoLogWriter(dy *dynamo.Service, wg *sync.WaitGroup) *DynamoLogWriter {
	return &DynamoLogWriter{
		Batcher: syncx.NewBatcher(func(batch []*clogs.Log) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			if err := writeDynamoChannelLogs(ctx, dy, batch); err != nil {
				slog.Error("error writing logs to dynamo", "error", err)
			}
		}, 25, time.Millisecond*500, 1000, wg),
	}
}

func writeDynamoChannelLogs(ctx context.Context, ds *dynamo.Service, batch []*clogs.Log) error {
	writeReqs := make([]types.WriteRequest, len(batch))

	for i, l := range batch {
		d, err := l.MarshalDynamo()
		if err != nil {
			return fmt.Errorf("error marshalling log for dynamo: %w", err)
		}
		writeReqs[i] = types.WriteRequest{PutRequest: &types.PutRequest{Item: d}}
	}

	resp, err := ds.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{ds.TableName("ChannelLogs"): writeReqs},
	})
	if err != nil {
		return err
	}
	if len(resp.UnprocessedItems) > 0 {
		// TODO shouldn't happend.. but need to figure out how we would retry these
		slog.Error("unprocessed items writing logs to dynamo", "count", len(resp.UnprocessedItems))
	}
	return nil
}
