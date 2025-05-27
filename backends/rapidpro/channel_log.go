package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
)

const (
	dynamoChannelLogTTL = 7 * 24 * time.Hour // 1 week
)

const sqlInsertChannelLog = `
INSERT INTO channels_channellog( uuid,  log_type,  channel_id,  http_logs,  errors,  is_error,  created_on,  elapsed_ms)
                         VALUES(:uuid, :log_type, :channel_id, :http_logs, :errors, :is_error, :created_on, :elapsed_ms)`

// channel log to be inserted into the database
type dbChannelLog struct {
	UUID      clogs.UUID        `db:"uuid"`
	Type      clogs.Type        `db:"log_type"`
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

	dynLog, err := NewDynamoChannelLog(clog)
	if err != nil {
		log.Error("error creating dynamo channel log", "error", err)
		return
	}

	if b.dyLogWriter.Queue(dynLog) <= 0 {
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

func NewDynamoChannelLog(clog *courier.ChannelLog) (*DynamoItem, error) {
	key := GetChannelLogKey(clog)

	type DataGZ struct {
		HttpLogs []*httpx.Log   `json:"http_logs"`
		Errors   []*clogs.Error `json:"errors"`
	}

	dataGZ, err := dynamo.MarshalJSONGZ(&DataGZ{HttpLogs: clog.HttpLogs, Errors: clog.Errors})
	if err != nil {
		return nil, fmt.Errorf("error encoding http logs as JSON+GZip: %w", err)
	}

	return &DynamoItem{
		DynamoKey: key,
		OrgID:     int(clog.Channel().(*Channel).OrgID()),
		TTL:       clog.CreatedOn.Add(dynamoChannelLogTTL),
		Data: map[string]any{
			"type":       clog.Type,
			"elapsed_ms": int(clog.Elapsed / time.Millisecond),
			"created_on": clog.CreatedOn,
		},
		DataGZ: dataGZ,
	}, nil
}

func GetChannelLogKey(l *courier.ChannelLog) DynamoKey {
	pk := fmt.Sprintf("cha#%s#%s", l.Channel().UUID(), l.UUID[len(l.UUID)-1:]) // 16 buckets for each channel
	sk := fmt.Sprintf("log#%s", l.UUID)
	return DynamoKey{PK: pk, SK: sk}
}
