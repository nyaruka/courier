package rapidpro

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dytypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
)

const sqlInsertChannelLog = `
INSERT INTO channels_channellog( uuid,  log_type,  channel_id,  http_logs,  errors,  is_error,  created_on,  elapsed_ms)
                         VALUES(:uuid, :log_type, :channel_id, :http_logs, :errors, :is_error, :created_on, :elapsed_ms)`

// channel log to be inserted into the database
type dbChannelLog struct {
	UUID      courier.ChannelLogUUID `db:"uuid"`
	Type      courier.ChannelLogType `db:"log_type"`
	ChannelID courier.ChannelID      `db:"channel_id"`
	HTTPLogs  json.RawMessage        `db:"http_logs"`
	Errors    json.RawMessage        `db:"errors"`
	IsError   bool                   `db:"is_error"`
	CreatedOn time.Time              `db:"created_on"`
	ElapsedMS int                    `db:"elapsed_ms"`
}

// channel log to be written to logs storage
type stChannelLog struct {
	UUID        courier.ChannelLogUUID `json:"uuid"`
	Type        courier.ChannelLogType `json:"type"`
	HTTPLogs    []*httpx.Log           `json:"http_logs"`
	Errors      []channelError         `json:"errors"`
	ElapsedMS   int                    `json:"elapsed_ms"`
	CreatedOn   time.Time              `json:"created_on"`
	ChannelUUID courier.ChannelUUID    `json:"-"`
}

func (l *stChannelLog) path() string {
	return path.Join("channels", string(l.ChannelUUID), string(l.UUID[:4]), fmt.Sprintf("%s.json", l.UUID))
}

// channel log to be written to DynamoDB
type dyChannelLog struct {
	UUID      courier.ChannelLogUUID `dynamodbav:"UUID"`
	Type      courier.ChannelLogType `dynamodbav:"Type"`
	DataGZ    []byte                 `dynamodbav:"DataGZ,omitempty"`
	ElapsedMS int                    `dynamodbav:"ElapsedMS"`
	CreatedOn time.Time              `dynamodbav:"CreatedOn,unixtime"`
	ExpiresOn time.Time              `dynamodbav:"ExpiresOn,unixtime"`
}

type channelError struct {
	Code    string `json:"code"`
	ExtCode string `json:"ext_code,omitempty"`
	Message string `json:"message"`
}

// queues the passed in channel log to a writer
func queueChannelLog(b *backend, clog *courier.ChannelLog) {
	log := slog.With("log_uuid", clog.UUID(), "log_type", clog.Type(), "channel_uuid", clog.Channel().UUID())
	dbChan := clog.Channel().(*Channel)

	// so that we don't save null
	logs := clog.HTTPLogs()
	if logs == nil {
		logs = []*httpx.Log{}
	}

	errors := make([]channelError, len(clog.Errors()))
	for i, e := range clog.Errors() {
		errors[i] = channelError{Code: e.Code(), ExtCode: e.ExtCode(), Message: e.Message()}
	}
	isError := clog.IsError()

	// depending on the channel log policy, we might be able to discard this log
	if dbChan.LogPolicy == LogPolicyNone || (dbChan.LogPolicy == LogPolicyErrors && !isError) {
		return
	}

	// save http logs and errors as gzipped JSON
	data := jsonx.MustMarshal(map[string]any{"http_logs": logs, "errors": errors})
	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)
	w.Write(data)
	w.Close()

	dl := &dyChannelLog{
		UUID:      clog.UUID(),
		Type:      clog.Type(),
		DataGZ:    buf.Bytes(),
		ElapsedMS: int(clog.Elapsed() / time.Millisecond),
		CreatedOn: clog.CreatedOn(),
		ExpiresOn: clog.CreatedOn().Add(14 * 24 * time.Hour),
	}
	if b.dyLogWriter.Queue(dl) <= 0 {
		log.With("storage", "dynamo").Error("channel log writer buffer full")
	}

	// if log is attached to a call or message, only write to storage
	if clog.Attached() {
		v := &stChannelLog{
			UUID:        clog.UUID(),
			Type:        clog.Type(),
			HTTPLogs:    logs,
			Errors:      errors,
			ElapsedMS:   int(clog.Elapsed() / time.Millisecond),
			CreatedOn:   clog.CreatedOn(),
			ChannelUUID: clog.Channel().UUID(),
		}
		if b.stLogWriter.Queue(v) <= 0 {
			log.With("storage", "s3").Error("channel log writer buffer full")
		}
	} else {
		// otherwise write to database so it's retrievable
		v := &dbChannelLog{
			UUID:      clog.UUID(),
			Type:      clog.Type(),
			ChannelID: dbChan.ID(),
			HTTPLogs:  jsonx.MustMarshal(logs),
			Errors:    jsonx.MustMarshal(errors),
			IsError:   isError,
			CreatedOn: clog.CreatedOn(),
			ElapsedMS: int(clog.Elapsed() / time.Millisecond),
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
				log := slog.With("comp", "log writer", "log_uuid", v.UUID)

				if qerr := dbutil.AsQueryError(err); qerr != nil {
					query, params := qerr.Query()
					log = log.With("sql", query, "sql_params", params)
				}

				log.Error("error writing channel log", "error", err)
			}
		}
	}
}

type StorageLogWriter struct {
	*syncx.Batcher[*stChannelLog]
}

func NewStorageLogWriter(s3s *s3x.Service, bucket string, wg *sync.WaitGroup) *StorageLogWriter {
	return &StorageLogWriter{
		Batcher: syncx.NewBatcher(func(batch []*stChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeStorageChannelLogs(ctx, s3s, bucket, batch)
		}, 1000, time.Millisecond*500, 1000, wg),
	}
}

func writeStorageChannelLogs(ctx context.Context, s3s *s3x.Service, bucket string, batch []*stChannelLog) {
	uploads := make([]*s3x.Upload, len(batch))
	for i, l := range batch {
		uploads[i] = &s3x.Upload{
			Bucket:      bucket,
			Key:         l.path(),
			ContentType: "application/json",
			Body:        jsonx.MustMarshal(l),
			ACL:         s3types.ObjectCannedACLPrivate,
		}
	}
	if err := s3s.BatchPut(ctx, uploads, 32); err != nil {
		slog.Error("error writing channel logs", "comp", "storage log writer")
	}
}

type DynamoLogWriter struct {
	*syncx.Batcher[*dyChannelLog]
}

func NewDynamoLogWriter(dy *dynamo.Service, wg *sync.WaitGroup) *DynamoLogWriter {
	return &DynamoLogWriter{
		Batcher: syncx.NewBatcher(func(batch []*dyChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeDynamoChannelLogs(ctx, dy, batch)
		}, 25, time.Millisecond*500, 1000, wg),
	}
}

func writeDynamoChannelLogs(ctx context.Context, dy *dynamo.Service, batch []*dyChannelLog) {
	log := slog.With("comp", "dynamo log writer")

	var writeReqs []dytypes.WriteRequest
	for _, l := range batch {
		item, err := attributevalue.MarshalMap(l)
		if err != nil {
			log.Error("error marshalling channel log", "error", err)
		} else {
			writeReqs = append(writeReqs, dytypes.WriteRequest{PutRequest: &dytypes.PutRequest{Item: item}})
		}
	}

	if len(writeReqs) > 0 {
		_, err := dy.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]dytypes.WriteRequest{dy.TableName("ChannelLogs"): writeReqs},
		})
		if err != nil {
			log.Error("error writing channel logs", "error", err)
		}
	}
}
