package handlers_test

import (
	"errors"
	"net/http"
	"testing"

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

func TestIsThrottled(t *testing.T) {
	assert.True(t, handlers.IsThrottled(&http.Response{StatusCode: 429}))
	assert.False(t, handlers.IsThrottled(&http.Response{StatusCode: 200}))
	assert.False(t, handlers.IsThrottled(&http.Response{StatusCode: 503}))
	assert.False(t, handlers.IsThrottled(nil))
}
