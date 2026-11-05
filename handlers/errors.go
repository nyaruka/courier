package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
)

// a Retry-After any longer than this is more likely a misconfigured provider than a real rate limit, so we don't let it
// hold back a channel for longer
const maxRetryAfter = 5 * time.Minute

// ErrorFromResponse converts the response and error from a send request into the send error that should be
// returned to the sender, or nil if the response indicates success. Note that HTTP 429 is treated as throttling
// for every channel type - it's a standard status code and no channel uses it to mean permanent failure - and holds
// back the whole channel if it has a Retry-After (see ThrottledError), so handlers for providers whose rate limits can
// apply to a single recipient should check IsThrottled first.
func ErrorFromResponse(resp *http.Response, err error) error {
	if err != nil || resp.StatusCode/100 == 5 {
		return channels.ErrConnectionFailed
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return ThrottledError(resp)
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

// ThrottledError returns the error for a rate limited response, which holds back the whole channel if the response says
// how long to wait with a Retry-After header. Handlers should only use this where the provider's rate limit applies to
// the whole channel rather than to the recipient.
func ThrottledError(resp *http.Response) error {
	if d := retryAfter(resp); d > 0 {
		return channels.ErrConnectionThrottledFor(min(d, maxRetryAfter))
	}
	return channels.ErrConnectionThrottled
}

// parses a Retry-After header, which can be a number of seconds or an HTTP date
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}
