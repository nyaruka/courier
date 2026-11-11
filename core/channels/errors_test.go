package channels_test

import (
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/stretchr/testify/assert"
)

func TestErrNotSent(t *testing.T) {
	err := channels.ErrNotSent(channels.ErrConnectionThrottled, 30*time.Second)
	assert.True(t, err.NotSent())
	assert.True(t, err.Retryable())
	assert.Equal(t, 30*time.Second, err.Backoff())
	assert.Equal(t, "connection_throttled", err.ClogError().Code)
	assert.Equal(t, 30*time.Second, channels.RequestedBackoff(&channels.SendResult{}, err))

	// the error it wraps is left as it was
	assert.False(t, channels.ErrConnectionThrottled.(*channels.SendError).NotSent())

	// it always pauses the channel, as putting the message back without a pause would just pop it again
	assert.Equal(t, time.Second, channels.ErrNotSent(channels.ErrConnectionThrottled, 0).Backoff())

	// a non-retryable error is made retryable, and anything else is taken as a failed connection
	assert.True(t, channels.ErrNotSent(channels.ErrResponseStatus, time.Second).Retryable())
	assert.Equal(t, "connection_failed", channels.ErrNotSent(assert.AnError, time.Second).ClogError().Code)
}
