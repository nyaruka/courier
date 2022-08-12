package courier

import (
	"fmt"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/httpx"
)

// NewChannelLog creates a new channel log for the passed in channel, id, and request and response info
func NewChannelLog(description string, channel Channel, msgID MsgID, method string, url string, statusCode int,
	request string, response string, elapsed time.Duration, err error) *ChannelLog {

	errString := ""
	if err != nil {
		errString = err.Error()
	}

	return &ChannelLog{
		Description: description,
		Channel:     channel,
		MsgID:       msgID,
		Method:      method,
		URL:         url,
		StatusCode:  statusCode,
		Error:       errString,
		Request:     SanitizeBody(request),
		Response:    SanitizeBody(response),
		CreatedOn:   time.Now(),
		Elapsed:     elapsed,
	}
}

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

// NewChannelLogFromTrace creates a new channel log for the passed in channel, id, and http trace
func NewChannelLogFromTrace(description string, channel Channel, msgID MsgID, trace *httpx.Trace) *ChannelLog {
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

// NewChannelLogFromError creates a new channel log for the passed in channel, msg id and error
func NewChannelLogFromError(description string, channel Channel, msgID MsgID, elapsed time.Duration, err error) *ChannelLog {
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
	ChannelLogTypeMessageSend    ChannelLogType = "message_send"
	ChannelLogTypeMessageReceive ChannelLogType = "message_receive"
)

var logTypeDescriptions = map[ChannelLogType]string{
	ChannelLogTypeMessageSend:    "Message Sent",
	ChannelLogTypeMessageReceive: "Message Received",
}
var logTypeErrorDescriptions = map[ChannelLogType]string{
	ChannelLogTypeMessageSend:    "Message Sending Error",
	ChannelLogTypeMessageReceive: "Message Receive Error",
}

type ChannelLogger struct {
	type_   ChannelLogType
	channel Channel
	msgID   MsgID

	errors []string
	logs   []*ChannelLog
}

func NewChannelLoggerForSend(msg Msg) *ChannelLogger {
	return &ChannelLogger{type_: ChannelLogTypeMessageSend, channel: msg.Channel(), msgID: msg.ID()}
}

func NewChannelLoggerForReceive(channel Channel) *ChannelLogger {
	return &ChannelLogger{type_: ChannelLogTypeMessageSend, channel: channel}
}

// HTTP logs an HTTP request and response
func (l *ChannelLogger) HTTP(t *httpx.Trace) {
	var description string
	if t.Response == nil || t.Response.StatusCode/100 != 2 {
		description = logTypeErrorDescriptions[l.type_]
	} else {
		description = logTypeDescriptions[l.type_]
	}

	l.logs = append(l.logs, NewChannelLogFromTrace(description, l.channel, l.msgID, t))
}

func (l *ChannelLogger) Error(err error) {
	l.errors = append(l.errors, err.Error())

	// if we have an existing log which isn't already an error, update it
	if len(l.logs) > 0 && l.logs[len(l.logs)-1].Error == "" {
		l.logs[len(l.logs)-1].Error = err.Error()
		l.logs[len(l.logs)-1].Description = logTypeErrorDescriptions[l.type_]
	} else {
		l.logs = append(l.logs, NewChannelLogFromError(logTypeErrorDescriptions[l.type_], l.channel, l.msgID, 0, err))
	}
}

func (l *ChannelLogger) Errors() []string {
	return l.errors
}

func (l *ChannelLogger) Logs() []*ChannelLog {
	return l.logs
}
