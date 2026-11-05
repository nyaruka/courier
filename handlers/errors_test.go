package handlers_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/stretchr/testify/assert"
)

func TestErrorFromResponse(t *testing.T) {
	tcs := []struct {
		status      int
		err         error
		expectedErr error
	}{
		{200, nil, nil},
		{201, nil, nil},
		{400, nil, channels.ErrResponseStatus},
		{401, nil, channels.ErrResponseStatus},
		{429, nil, channels.ErrConnectionThrottled},
		{500, nil, channels.ErrConnectionFailed},
		{503, nil, channels.ErrConnectionFailed},
		{0, errors.New("timeout"), channels.ErrConnectionFailed},
	}

	for _, tc := range tcs {
		var resp *http.Response
		if tc.err == nil {
			resp = &http.Response{StatusCode: tc.status}
		}

		assert.Equal(t, tc.expectedErr, handlers.ErrorFromResponse(resp, tc.err), "mismatch for status %d", tc.status)
	}
}

func TestThrottledError(t *testing.T) {
	throttled := func(retryAfter string) error {
		resp := &http.Response{StatusCode: 429, Header: http.Header{}}
		if retryAfter != "" {
			resp.Header.Set("Retry-After", retryAfter)
		}
		return handlers.ErrorFromResponse(resp, nil)
	}
	backoff := func(err error) time.Duration {
		return channels.RequestedBackoff(&channels.SendResult{}, err)
	}

	assert.Equal(t, channels.ErrConnectionThrottled, throttled(""))
	assert.Equal(t, time.Duration(0), backoff(throttled("")))
	assert.Equal(t, time.Duration(0), backoff(throttled("soon")))
	assert.Equal(t, time.Duration(0), backoff(throttled("Wed, 21 Oct 2015 07:28:00 GMT"))) // in the past
	assert.Equal(t, 30*time.Second, backoff(throttled("30")))
	assert.Equal(t, 5*time.Minute, backoff(throttled("3600"))) // capped

	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	assert.InDelta(t, 2*time.Minute, backoff(throttled(future)), float64(2*time.Second))

	// a throttled error with a backoff is still a throttled error
	assert.Equal(t, "channel rate limited", throttled("30").Error())
	assert.Equal(t, channels.ErrConnectionThrottled.(*channels.SendError).ClogError(), throttled("30").(*channels.SendError).ClogError())

	// and a result's backoff is used if longer
	res := &channels.SendResult{}
	res.SetBackoff(time.Minute)
	assert.Equal(t, time.Minute, channels.RequestedBackoff(res, throttled("30")))
	assert.Equal(t, time.Minute, channels.RequestedBackoff(res, nil))
}

func TestIsThrottled(t *testing.T) {
	assert.True(t, handlers.IsThrottled(&http.Response{StatusCode: 429}))
	assert.False(t, handlers.IsThrottled(&http.Response{StatusCode: 200}))
	assert.False(t, handlers.IsThrottled(&http.Response{StatusCode: 503}))
	assert.False(t, handlers.IsThrottled(nil))
}
