package rapidpro

import (
	"context"
	"fmt"

	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
)

const insertLogSQL = `
INSERT INTO 
	channels_channellog("channel_id", "msg_id", "description", "is_error", "method", "url", "request", "response", "response_status", "created_on", "request_time")
                 VALUES(:channel_id,  :msg_id,  :description,  :is_error,  :method,  :url,  :request,  :response,  :response_status,  :created_on,  :request_time)
`

// ChannelLog is our DB specific struct for logs
type ChannelLog struct {
	ChannelID      courier.ChannelID `db:"channel_id"`
	MsgID          courier.MsgID     `db:"msg_id"`
	Description    string            `db:"description"`
	IsError        bool              `db:"is_error"`
	Method         string            `db:"method"`
	URL            string            `db:"url"`
	Request        string            `db:"request"`
	Response       string            `db:"response"`
	ResponseStatus int               `db:"response_status"`
	CreatedOn      time.Time         `db:"created_on"`
	RequestTime    int               `db:"request_time"`
}

// RowID satisfies our batch.Value interface, we are always inserting logs so we have no row id
func (l *ChannelLog) RowID() string {
	return ""
}

// WriteChannelLog writes the passed in channel log to the database, we do not queue on errors but instead just throw away the log
func writeChannelLog(ctx context.Context, b *backend, log *courier.ChannelLog) error {
	// cast our channel to our own channel type
	dbChan, isChan := log.Channel.(*DBChannel)
	if !isChan {
		return fmt.Errorf("unable to write non-rapidpro channel logs")
	}

	// if we have an error, append to to our response
	if log.Error != "" {
		log.Response += "\n\nError: " + log.Error
	}

	// strip null chars from request and response, postgres doesn't like that
	log.Request = utils.CleanString(log.Request)
	log.Response = utils.CleanString(log.Response)

	// create our value for committing
	v := &ChannelLog{
		ChannelID:      dbChan.ID(),
		MsgID:          log.MsgID,
		Description:    log.Description,
		IsError:        log.Error != "",
		Method:         log.Method,
		URL:            log.URL,
		Request:        log.Request,
		Response:       log.Response,
		ResponseStatus: log.StatusCode,
		CreatedOn:      log.CreatedOn,
		RequestTime:    int(log.Elapsed / time.Millisecond),
	}

	// queue it
	b.logCommitter.Queue(v)
	return nil
}
