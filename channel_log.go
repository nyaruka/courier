package courier

import (
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
)

// ChannelLogType is the type of channel interaction we are logging
type ChannelLogType string

const (
	ChannelLogTypeUnknown       ChannelLogType = "unknown"
	ChannelLogTypeMsgSend       ChannelLogType = "msg_send"
	ChannelLogTypeMsgStatus     ChannelLogType = "msg_status"
	ChannelLogTypeMsgReceive    ChannelLogType = "msg_receive"
	ChannelLogTypeEventReceive  ChannelLogType = "event_receive"
	ChannelLogTypeTokenFetch    ChannelLogType = "token_fetch"
	ChannelLogTypePageSubscribe ChannelLogType = "page_subscribe"
)

type ChannelError struct {
	message string
	code    string
}

func NewChannelError(message, code string) ChannelError {
	return ChannelError{message: message, code: code}
}

func (e *ChannelError) Message() string {
	return e.message
}

func (e *ChannelError) Code() string {
	return e.code
}

type ChannelLog struct {
	type_     ChannelLogType
	channel   Channel
	msgID     MsgID
	recorder  *httpx.Recorder
	httpLogs  []*httpx.Log
	errors    []ChannelError
	createdOn time.Time
	elapsed   time.Duration
}

func NewChannelLogForIncoming(r *httpx.Recorder, channel Channel) *ChannelLog {
	return &ChannelLog{type_: ChannelLogTypeUnknown, recorder: r, channel: channel, createdOn: dates.Now()}
}

func NewChannelLogForSend(msg Msg) *ChannelLog {
	return &ChannelLog{type_: ChannelLogTypeMsgSend, channel: msg.Channel(), msgID: msg.ID(), createdOn: dates.Now()}
}

func NewChannelLog(t ChannelLogType, channel Channel) *ChannelLog {
	return &ChannelLog{type_: t, channel: channel, createdOn: dates.Now()}
}

// HTTP logs an outgoing HTTP request and response
func (l *ChannelLog) HTTP(t *httpx.Trace) {
	l.httpLogs = append(l.httpLogs, l.traceToLog(t))
}

func (l *ChannelLog) Error(err error) {
	l.errors = append(l.errors, NewChannelError(err.Error(), ""))
}

func (l *ChannelLog) End() {
	if l.recorder != nil {
		// prepend so it's the first HTTP request in the log
		l.httpLogs = append([]*httpx.Log{l.traceToLog(l.recorder.Trace)}, l.httpLogs...)
	}

	l.elapsed = time.Since(l.createdOn)
}

func (l *ChannelLog) Type() ChannelLogType {
	return l.type_
}

func (l *ChannelLog) SetType(t ChannelLogType) {
	l.type_ = t
}

func (l *ChannelLog) Channel() Channel {
	return l.channel
}

func (l *ChannelLog) MsgID() MsgID {
	return l.msgID
}

func (l *ChannelLog) SetMsgID(id MsgID) {
	l.msgID = id
}

func (l *ChannelLog) HTTPLogs() []*httpx.Log {
	return l.httpLogs
}

func (l *ChannelLog) Errors() []ChannelError {
	return l.errors
}

func (l *ChannelLog) CreatedOn() time.Time {
	return l.createdOn
}

func (l *ChannelLog) Elapsed() time.Duration {
	return l.elapsed
}

func (l *ChannelLog) traceToLog(t *httpx.Trace) *httpx.Log {
	return httpx.NewLog(t, 2048, 50000, nil)
}
