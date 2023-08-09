package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/sirupsen/logrus"
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

type channelError struct {
	Code    string `json:"code"`
	ExtCode string `json:"ext_code,omitempty"`
	Message string `json:"message"`
}

// queues the passed in channel log to a writer
func queueChannelLog(ctx context.Context, b *backend, clog *courier.ChannelLog) {
	dbChan := clog.Channel().(*DBChannel)

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

	// if log is attached to a call or message, only write to storage
	if clog.MsgID() != courier.NilMsgID {
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
			logrus.Error("channel log storage writer buffer full")
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
			logrus.Error("channel log db writer buffer full")
		}
	}
}

type DBLogWriter struct {
	*syncx.Batcher[*dbChannelLog]
}

func NewDBLogWriter(db *sqlx.DB, wg *sync.WaitGroup) *DBLogWriter {
	return &DBLogWriter{
		Batcher: syncx.NewBatcher[*dbChannelLog](func(batch []*dbChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeDBChannelLogs(ctx, db, batch)
		}, time.Millisecond*500, 1000, wg),
	}
}

func writeDBChannelLogs(ctx context.Context, db *sqlx.DB, logs []*dbChannelLog) {
	for _, batch := range utils.ChunkSlice(logs, 1000) {
		err := dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, batch)

		// if we received an error, try again one at a time (in case it is one value hanging us up)
		if err != nil {
			for _, v := range batch {
				err = dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, []*dbChannelLog{v})
				if err != nil {
					log := logrus.WithField("comp", "log writer").WithField("log_uuid", v.UUID)

					if qerr := dbutil.AsQueryError(err); qerr != nil {
						query, params := qerr.Query()
						log = log.WithFields(logrus.Fields{"sql": query, "sql_params": params})
					}

					log.WithError(err).Error("error writing channel log")
				}
			}
		}
	}
}

type StorageLogWriter struct {
	*syncx.Batcher[*stChannelLog]
}

func NewStorageLogWriter(st storage.Storage, wg *sync.WaitGroup) *StorageLogWriter {
	return &StorageLogWriter{
		Batcher: syncx.NewBatcher[*stChannelLog](func(batch []*stChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeStorageChannelLogs(ctx, st, batch)
		}, time.Millisecond*500, 1000, wg),
	}
}

func writeStorageChannelLogs(ctx context.Context, st storage.Storage, logs []*stChannelLog) {
	uploads := make([]*storage.Upload, len(logs))
	for i, l := range logs {
		uploads[i] = &storage.Upload{
			Path:        l.path(),
			ContentType: "application/json",
			Body:        jsonx.MustMarshal(l),
		}
	}
	if err := st.BatchPut(ctx, uploads); err != nil {
		logrus.WithField("comp", "storage log writer").Error("error writing channel logs")
	}
}
