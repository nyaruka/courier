package handlers

import (
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
)

// ErrorFromResponse converts the response and error from a send request into the send error that should be
// returned to the sender, or nil if the response indicates success. Note that HTTP 429 is treated as throttling
// for every channel type - it's a standard status code and no channel uses it to mean permanent failure.
func ErrorFromResponse(resp *http.Response, err error) error {
	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return channels.ErrConnectionThrottled
	}
	if resp.StatusCode/100 != 2 {
		return channels.ErrResponseStatus
	}
	return nil
}

// IsThrottled returns whether the given response indicates that we're being rate limited. Handlers which need to
// inspect the response body before deciding how to fail should use this to check the status code first.
func IsThrottled(resp *http.Response) bool {
	return resp != nil && resp.StatusCode == http.StatusTooManyRequests
}
