package channels

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
)

// IgnoredRequest is returned by a receive function for a request we understood but which asked nothing of us -
// a status we don't track, an echo of our own message. It isn't a failure, so it's answered as ignored rather
// than as an error, and the details say what we saw.
type IgnoredRequest struct {
	Details string
}

func (e *IgnoredRequest) Error() string { return e.Details }

// Ignore returns an error saying this request asked nothing of us, and why
func Ignore(format string, args ...any) error {
	return &IgnoredRequest{Details: fmt.Sprintf(format, args...)}
}

// UnauthenticatedRequest is returned by a receive function for a request that didn't prove it came from the
// provider - a missing or wrong auth header. It's answered as unauthorized rather than as a bad request.
type UnauthenticatedRequest struct {
	Err error
}

func (e *UnauthenticatedRequest) Error() string { return e.Err.Error() }

// Unauthenticated returns an error saying this request didn't prove where it came from
func Unauthenticated(err error) error { return &UnauthenticatedRequest{Err: err} }

// RequestReply is returned by a receive function whose provider dictates the body this particular request
// must be answered with - a verification challenge echoed back, a welcome message. Anything the batch
// gathered is still written first; the reply replaces only the standard success response.
type RequestReply struct {
	ContentType string
	Body        []byte
}

func (e *RequestReply) Error() string { return "request answered with its own reply" }

// Write writes the reply to the given response writer
func (e *RequestReply) Write(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", e.ContentType)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(e.Body)
	return err
}

// Reply returns an error saying this request must be answered with the given body
func Reply(contentType string, body []byte) error {
	return &RequestReply{ContentType: contentType, Body: body}
}

type SendResult struct {
	externalIDs []string
	otherURNs   []urns.URN
	backoff     time.Duration
	cost        int
}

func (r *SendResult) AddExternalID(id string) {
	r.externalIDs = append(r.externalIDs, id)
}

func (r *SendResult) ExternalIDs() []string {
	return r.externalIDs
}

// AddOtherURN adds a URN the provider reported for the recipient, to be added to their contact if nobody else owns it
func (r *SendResult) AddOtherURN(urn urns.URN) {
	r.otherURNs = append(r.otherURNs, urn)
}

func (r *SendResult) OtherURNs() []urns.URN {
	return r.otherURNs
}

// SetBackoff asks for all sending on the channel to stop for the given duration, e.g. because the provider can't take
// any more for now. It applies whether or not this send succeeded, and unlike ErrConnectionThrottled, which may only
// apply to this message or its recipient, it holds back every message on the channel.
func (r *SendResult) SetBackoff(d time.Duration) {
	r.backoff = d
}

func (r *SendResult) Backoff() time.Duration {
	return r.backoff
}

// AddCost adds to what the send used of the channel's throughput, in the units the provider limits it by, e.g.
// segments for an SMS aggregator or messages for a chat API. Handlers add it as the provider accepts each message, so
// a send that fails partway costs only what got through.
func (r *SendResult) AddCost(n int) {
	r.cost += n
}

func (r *SendResult) Cost() int {
	return r.cost
}

// RequestedBackoff returns how long a send asked for all sending on the channel to stop, through its result or its error
func RequestedBackoff(res *SendResult, err error) time.Duration {
	var serr *SendError
	if errors.As(err, &serr) {
		return max(res.Backoff(), serr.Backoff())
	}
	return res.Backoff()
}

type SendError struct {
	msg       string
	retryable bool
	loggable  bool

	clogCode    string
	clogMsg     string
	clogExtCode string

	backoff time.Duration
}

func (e *SendError) Error() string {
	return e.msg
}

// Retryable returns whether a send which failed this way should be tried again later
func (e *SendError) Retryable() bool { return e.retryable }

// Loggable returns whether this failure indicates a problem on our side worth logging as an error
func (e *SendError) Loggable() bool { return e.loggable }

// Backoff returns how long all sending on the channel should stop after a send failed this way
func (e *SendError) Backoff() time.Duration { return e.backoff }

// ClogError returns the user facing error to record on the channel log
func (e *SendError) ClogError() *svclogs.Error {
	return &svclogs.Error{Code: e.clogCode, ExtCode: e.clogExtCode, Message: e.clogMsg}
}

// ErrChannelConfig should be returned by a handler send method when channel config is invalid
var ErrChannelConfig error = &SendError{
	msg:       "channel config invalid",
	retryable: false,
	loggable:  true,
	clogCode:  "channel_config",
	clogMsg:   "Channel configuration is missing required values.",
}

// ErrMessageInvalid should be returned by a handler send method when the message it has received is invalid
var ErrMessageInvalid error = &SendError{
	msg:       "message invalid",
	retryable: false,
	loggable:  true,
	clogCode:  "message_invalid",
	clogMsg:   "Message is missing required values.",
}

// ErrConnectionFailed should be returned when connection to the channel fails (timeout or 5XX response)
var ErrConnectionFailed error = &SendError{
	msg:       "channel connection failed",
	retryable: true,
	loggable:  false,
	clogCode:  "connection_failed",
	clogMsg:   "Connection to server failed.",
}

// ErrConnectionThrottled should be returned when channel tells us we're rate limited
var ErrConnectionThrottled error = &SendError{
	msg:       "channel rate limited",
	retryable: true,
	loggable:  false,
	clogCode:  "connection_throttled",
	clogMsg:   "Connection to server has been rate limited.",
}

// ErrConnectionThrottledFor should be returned when channel tells us we're rate limited and for how long, which unlike
// ErrConnectionThrottled holds back every message on the channel for that long
func ErrConnectionThrottledFor(d time.Duration) *SendError {
	err := *ErrConnectionThrottled.(*SendError)
	err.backoff = d
	return &err
}

// ErrResponseStatus should be returned when the response from the channel has a non-success status code
var ErrResponseStatus error = &SendError{
	msg:       "response status code",
	retryable: false,
	loggable:  false,
	clogCode:  "response_status",
	clogMsg:   "Response has non-success status code.",
}

// ErrResponseContent should be returned when the response content from the channel indicates non-succeess
var ErrResponseContent error = &SendError{
	msg:       "response content",
	retryable: false,
	loggable:  false,
	clogCode:  "response_content",
	clogMsg:   "Response content indicates non-success.",
}

// ErrResponseUnparseable should be returned when channel response can't be parsed in expected format
var ErrResponseUnparseable error = &SendError{
	msg:       "response couldn't be parsed",
	retryable: false,
	loggable:  true,
	clogCode:  "response_unparseable",
	clogMsg:   "Response could not be parsed in the expected format.",
}

// ErrResponseUnexpected should be returned when channel response doesn't match what we expect
var ErrResponseUnexpected error = &SendError{
	msg:       "response not expected values",
	retryable: false,
	loggable:  true,
	clogCode:  "response_unexpected",
	clogMsg:   "Response doesn't match expected values.",
}

// ErrContactStopped should be returned when channel tells us explicitly that the contact has opted-out
var ErrContactStopped error = &SendError{
	msg:       "contact opted out",
	retryable: false,
	loggable:  false,
	clogCode:  "contact_stopped",
	clogMsg:   "Contact has opted-out of messages from this channel.",
}

func ErrFailedWithReason(code, desc string) *SendError {
	return &SendError{
		msg:         "channel rejected send with reason",
		retryable:   false,
		loggable:    false,
		clogCode:    "rejected_with_reason",
		clogMsg:     desc,
		clogExtCode: code,
	}
}

// ErrRetryableWithReason is like ErrFailedWithReason but for failures that may be transient and so should be retried
func ErrRetryableWithReason(code, desc string) *SendError {
	return &SendError{
		msg:         "channel rejected send with retryable reason",
		retryable:   true,
		loggable:    false,
		clogCode:    "rejected_with_reason",
		clogMsg:     desc,
		clogExtCode: code,
	}
}
