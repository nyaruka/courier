package courier

import (
	"fmt"

	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	ChannelLogTypeUnknown         clogs.LogType = "unknown"
	ChannelLogTypeMsgSend         clogs.LogType = "msg_send"
	ChannelLogTypeMsgStatus       clogs.LogType = "msg_status"
	ChannelLogTypeMsgReceive      clogs.LogType = "msg_receive"
	ChannelLogTypeEventReceive    clogs.LogType = "event_receive"
	ChannelLogTypeMultiReceive    clogs.LogType = "multi_receive"
	ChannelLogTypeAttachmentFetch clogs.LogType = "attachment_fetch"
	ChannelLogTypeTokenRefresh    clogs.LogType = "token_refresh"
	ChannelLogTypePageSubscribe   clogs.LogType = "page_subscribe"
	ChannelLogTypeWebhookVerify   clogs.LogType = "webhook_verify"
)

func ErrorResponseStatusCode() *clogs.LogError {
	return &clogs.LogError{Code: "response_status_code", Message: "Unexpected response status code."}
}

func ErrorResponseUnparseable(format string) *clogs.LogError {
	return &clogs.LogError{Code: "response_unparseable", Message: fmt.Sprintf("Unable to parse response as %s.", format)}
}

func ErrorResponseUnexpected(expected string) *clogs.LogError {
	return &clogs.LogError{Code: "response_unexpected", Message: fmt.Sprintf("Expected response to be '%s'.", expected)}
}

func ErrorResponseValueMissing(key string) *clogs.LogError {
	return &clogs.LogError{Code: "response_value_missing", Message: fmt.Sprintf("Unable to find '%s' response.", key)}
}

func ErrorMediaUnsupported(contentType string) *clogs.LogError {
	return &clogs.LogError{Code: "media_unsupported", Message: fmt.Sprintf("Unsupported attachment media type: %s.", contentType)}
}

// ErrorMediaUnresolveable is used when media is unresolveable due to the channel's specific requirements
func ErrorMediaUnresolveable(contentType string) *clogs.LogError {
	return &clogs.LogError{Code: "media_unresolveable", Message: fmt.Sprintf("Unable to find version of %s attachment compatible with channel.", contentType)}
}

func ErrorAttachmentNotDecodable() *clogs.LogError {
	return &clogs.LogError{Code: "attachment_not_decodable", Message: "Unable to decode embedded attachment data."}
}

func ErrorExternal(code, message string) *clogs.LogError {
	if message == "" {
		message = fmt.Sprintf("Service specific error: %s.", code)
	}
	return &clogs.LogError{Code: "external", ExtCode: code, Message: message}
}

// ChannelLog stores the HTTP traces and errors generated by an interaction with a channel.
type ChannelLog struct {
	*clogs.Log

	channel  Channel
	attached bool
}

// NewChannelLogForIncoming creates a new channel log for an incoming request, the type of which won't be known
// until the handler completes.
func NewChannelLogForIncoming(logType clogs.LogType, ch Channel, r *httpx.Recorder, redactVals []string) *ChannelLog {
	return newChannelLog(logType, ch, r, false, redactVals)
}

// NewChannelLogForSend creates a new channel log for a message send
func NewChannelLogForSend(msg MsgOut, redactVals []string) *ChannelLog {
	return newChannelLog(ChannelLogTypeMsgSend, msg.Channel(), nil, true, redactVals)
}

// NewChannelLogForSend creates a new channel log for an attachment fetch
func NewChannelLogForAttachmentFetch(ch Channel, redactVals []string) *ChannelLog {
	return newChannelLog(ChannelLogTypeAttachmentFetch, ch, nil, true, redactVals)
}

// NewChannelLog creates a new channel log with the given type and channel
func NewChannelLog(t clogs.LogType, ch Channel, redactVals []string) *ChannelLog {
	return newChannelLog(t, ch, nil, false, redactVals)
}

func newChannelLog(t clogs.LogType, ch Channel, r *httpx.Recorder, attached bool, redactVals []string) *ChannelLog {
	return &ChannelLog{
		Log:      clogs.NewLog(t, r, redactVals),
		channel:  ch,
		attached: attached,
	}
}

// Deprecated: channel handlers should add user-facing error messages via .Error() instead
func (l *ChannelLog) RawError(err error) {
	l.Error(&clogs.LogError{Message: err.Error()})
}

func (l *ChannelLog) Channel() Channel {
	return l.channel
}

func (l *ChannelLog) Attached() bool {
	return l.attached
}

func (l *ChannelLog) SetAttached(a bool) {
	l.attached = a
}

// if we have an error or a non 2XX/3XX http response then log is considered an error
func (l *ChannelLog) IsError() bool {
	if len(l.Errors) > 0 {
		return true
	}

	for _, l := range l.HttpLogs {
		if l.StatusCode < 200 || l.StatusCode >= 400 {
			return true
		}
	}

	return false
}
