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
                 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`

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

	_, err := b.db.ExecContext(ctx, insertLogSQL, dbChan.ID(), log.MsgID, log.Description, log.Error != "", log.Method, log.URL,
		log.Request, log.Response, log.StatusCode, log.CreatedOn, log.Elapsed/time.Millisecond)

	return err
}
