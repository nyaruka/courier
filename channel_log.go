package courier

import (
	"fmt"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
)

func SanitizeBody(body string) string {
	parts := strings.SplitN(body, "\r\n\r\n", 2)
	if len(parts) < 2 {
		return body
	}

	ct := httpx.DetectContentType([]byte(parts[1]))

	// if this isn't text, replace with placeholder
	if !strings.HasPrefix(ct, "text") && !strings.HasPrefix(ct, "application/json") {
		return fmt.Sprintf("%s\r\n\r\nOmitting non text body of type: %s", parts[0], ct)
	}

	return body
}

// NewLegacyChannelLog creates a new channel log for the passed in channel, id, and http trace
func NewLegacyChannelLog(description string, channel Channel, msgID MsgID, trace *httpx.Trace) *ChannelLog {
	log := &ChannelLog{
		Description: description,
		Channel:     channel,
		MsgID:       msgID,
		Method:      trace.Request.Method,
		URL:         trace.Request.URL.String(),
		Request:     SanitizeBody(string(trace.RequestTrace)),
		CreatedOn:   trace.StartTime,
		Elapsed:     trace.EndTime.Sub(trace.StartTime),
	}

	if trace.Response != nil {
		log.StatusCode = trace.Response.StatusCode
		log.Response = string(trace.SanitizedResponse("..."))
	}

	return log
}

func newLegacyChannelLogFromError(description string, channel Channel, msgID MsgID, elapsed time.Duration, err error) *ChannelLog {
	return &ChannelLog{
		Description: description,
		Channel:     channel,
		MsgID:       msgID,
		Error:       err.Error(),
		CreatedOn:   time.Now(),
		Elapsed:     elapsed,
	}
}

// WithError augments the passed in ChannelLog with the passed in description and error if error is not nil
func (l *ChannelLog) WithError(description string, err error) *ChannelLog {
	if err != nil {
		l.Error = err.Error()
		l.Description = description
	}

	return l
}

func (l *ChannelLog) String() string {
	return fmt.Sprintf("%s: %d %s %d\n%s\n%s\n%s", l.Description, l.StatusCode, l.URL, l.Elapsed, l.Error, l.Request, l.Response)
}

// ChannelLog represents the log for a msg being received, sent or having its status updated. It includes the HTTP request
// and response for the action as well as the channel it was performed on and an option ID of the msg (for some error
// cases we may log without a msg id)
type ChannelLog struct {
	Description string
	Channel     Channel
	MsgID       MsgID
	Method      string
	URL         string
	StatusCode  int
	Error       string
	Request     string
	Response    string
	Elapsed     time.Duration
	CreatedOn   time.Time
}

// ChannelLogType is the type of channel interaction we are logging
type ChannelLogType string

const (
	ChannelLogTypeUnknown      ChannelLogType = "unknown"
	ChannelLogTypeMsgSend      ChannelLogType = "msg_send"
	ChannelLogTypeMsgStatus    ChannelLogType = "msg_status"
	ChannelLogTypeMsgReceive   ChannelLogType = "msg_receive"
	ChannelLogTypeEventReceive ChannelLogType = "event_receive"
	ChannelLogTypeTokenFetch   ChannelLogType = "token_fetch"
)

var logTypeDescriptions = map[ChannelLogType]string{
	ChannelLogTypeUnknown:      "Other Event",
	ChannelLogTypeMsgSend:      "Message Send",
	ChannelLogTypeMsgStatus:    "Message Status",
	ChannelLogTypeMsgReceive:   "Message Receive",
	ChannelLogTypeEventReceive: "Event Receive",
	ChannelLogTypeTokenFetch:   "Token Fetch",
}

type ChannelLogger struct {
	type_   ChannelLogType
	channel Channel
	msgID   MsgID

	traces    []*httpx.Trace
	errors    []string
	createdOn time.Time

	// deprecated
	logs []*ChannelLog
}

func NewChannelLogForSend(msg Msg) *ChannelLogger {
	return &ChannelLogger{type_: ChannelLogTypeMsgSend, channel: msg.Channel(), msgID: msg.ID(), createdOn: dates.Now()}
}

func NewChannelLog(t ChannelLogType, channel Channel) *ChannelLogger {
	return &ChannelLogger{type_: t, channel: channel, createdOn: dates.Now()}
}

func (l *ChannelLogger) SetType(t ChannelLogType) {
	l.type_ = t
}

func (l *ChannelLogger) SetMsgID(id MsgID) {
	l.msgID = id
}

// Recorder logs a recording of an incoming HTTP request
func (l *ChannelLogger) Recorder(r *httpx.Recorder) {
	// prepend so it's the first HTTP request in the log
	l.traces = append([]*httpx.Trace{r.Trace}, l.traces...)

	if l.channel != nil {
		l.logs = append(l.logs, NewLegacyChannelLog(logTypeDescriptions[l.type_], l.channel, l.msgID, r.Trace))
	}
}

// HTTP logs an outgoing HTTP request and response
func (l *ChannelLogger) HTTP(t *httpx.Trace) {
	l.traces = append(l.traces, t)

	if l.channel != nil {
		l.logs = append(l.logs, NewLegacyChannelLog(logTypeDescriptions[l.type_], l.channel, l.msgID, t))
	}
}

func (l *ChannelLogger) Error(err error) {
	l.errors = append(l.errors, err.Error())

	if l.channel != nil {
		// if we have an existing log which isn't already an error, update it
		if len(l.logs) > 0 && l.logs[len(l.logs)-1].Error == "" {
			l.logs[len(l.logs)-1].Error = err.Error()
		} else {
			l.logs = append(l.logs, newLegacyChannelLogFromError(logTypeDescriptions[l.type_], l.channel, l.msgID, 0, err))
		}
	}
}

func (l *ChannelLogger) Type() ChannelLogType {
	return l.type_
}

func (l *ChannelLogger) Traces() []*httpx.Trace {
	return l.traces
}

func (l *ChannelLogger) Errors() []string {
	return l.errors
}

func (l *ChannelLogger) LegacyLogs() []*ChannelLog {
	return l.logs
}

func (l *ChannelLogger) CreatedOn() time.Time {
	return l.createdOn
}
