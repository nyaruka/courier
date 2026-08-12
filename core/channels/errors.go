package channels

import (
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
)

type SendResult struct {
	externalIDs []string
	newURN      urns.URN
}

func (r *SendResult) AddExternalID(id string) {
	r.externalIDs = append(r.externalIDs, id)
}

func (r *SendResult) ExternalIDs() []string {
	return r.externalIDs
}

func (r *SendResult) SetNewURN(urn urns.URN) {
	r.newURN = urn
}

func (r *SendResult) NewURN() urns.URN {
	return r.newURN
}

type SendError struct {
	msg       string
	retryable bool
	loggable  bool

	clogCode    string
	clogMsg     string
	clogExtCode string
}

func (e *SendError) Error() string {
	return e.msg
}

// Retryable returns whether a send which failed this way should be tried again later
func (e *SendError) Retryable() bool { return e.retryable }

// Loggable returns whether this failure indicates a problem on our side worth logging as an error
func (e *SendError) Loggable() bool { return e.loggable }

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
