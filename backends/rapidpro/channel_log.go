package rapidpro

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/sirupsen/logrus"
)

const sqlInsertChannelLog = `
INSERT INTO channels_channellog( uuid,  log_type,  channel_id,  http_logs,  errors,  is_error,  created_on,  elapsed_ms)
                         VALUES(:uuid, :log_type, :channel_id, :http_logs, :errors, :is_error, :created_on, :elapsed_ms)`

// ChannelLog is our DB specific struct for logs
type ChannelLog struct {
	UUID      courier.ChannelLogUUID `db:"uuid"`
	Type      courier.ChannelLogType `db:"log_type"`
	ChannelID courier.ChannelID      `db:"channel_id"`
	HTTPLogs  json.RawMessage        `db:"http_logs"`
	Errors    json.RawMessage        `db:"errors"`
	IsError   bool                   `db:"is_error"`
	CreatedOn time.Time              `db:"created_on"`
	ElapsedMS int                    `db:"elapsed_ms"`
}

type channelError struct {
	Code    string `json:"code"`
	ExtCode string `json:"ext_code,omitempty"`
	Message string `json:"message"`
}

// queues the passed in channel log the committer, we do not queue on errors but instead just throw away the log
func queueChannelLog(ctx context.Context, b *backend, clog *courier.ChannelLog) error {
	dbChan := clog.Channel().(*DBChannel)

	// if we have an error or a non 2XX/3XX http response then this log is marked as an error
	isError := len(clog.Errors()) > 0
	if !isError {
		for _, l := range clog.HTTPLogs() {
			if l.StatusCode < 200 || l.StatusCode >= 400 {
				isError = true
				break
			}
		}
	}

	errors := make([]channelError, len(clog.Errors()))
	for i, e := range clog.Errors() {
		errors[i] = channelError{Code: e.Code(), ExtCode: e.ExtCode(), Message: e.Message()}
	}

	// create our value for committing
	v := &ChannelLog{
		UUID:      clog.UUID(),
		Type:      clog.Type(),
		ChannelID: dbChan.ID(),
		HTTPLogs:  jsonx.MustMarshal(clog.HTTPLogs()),
		Errors:    jsonx.MustMarshal(errors),
		IsError:   isError,
		CreatedOn: clog.CreatedOn(),
		ElapsedMS: int(clog.Elapsed() / time.Millisecond),
	}

	// queue it
	if b.logWriter.Queue(v) <= 0 {
		logrus.Error("channel log buffer full")
	}
	return nil
}

type LogWriter struct {
	*syncx.Batcher[*ChannelLog]
}

func NewLogWriter(db *sqlx.DB, wg *sync.WaitGroup) *LogWriter {
	return &LogWriter{
		Batcher: syncx.NewBatcher[*ChannelLog](func(batch []*ChannelLog) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeChannelLogs(ctx, db, batch)
		}, time.Millisecond*500, 1000, wg),
	}
}

func writeChannelLogs(ctx context.Context, db *sqlx.DB, logs []*ChannelLog) {
	for _, batch := range utils.ChunkSlice(logs, 1000) {
		err := dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, batch)

		// if we received an error, try again one at a time (in case it is one value hanging us up)
		if err != nil {
			for _, v := range batch {
				err = dbutil.BulkQuery(ctx, db, sqlInsertChannelLog, []*ChannelLog{v})
				if err != nil {
					log := logrus.WithField("comp", "log committer").WithField("log_uuid", v.UUID)

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
