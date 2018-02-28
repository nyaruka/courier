package courier

import (
	"fmt"
	"time"

	"github.com/nyaruka/courier/utils"
)

// NilStatusCode is used when we have an error before even sending anything
const NilStatusCode int = 417

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
		Request:     request,
		Response:    response,
		CreatedOn:   time.Now(),
		Elapsed:     elapsed,
	}
}

// NewChannelLogFromRR creates a new channel log for the passed in channel, id, and request/response log
func NewChannelLogFromRR(description string, channel Channel, msgID MsgID, rr *utils.RequestResponse) *ChannelLog {
	log := &ChannelLog{
		Description: description,
		Channel:     channel,
		MsgID:       msgID,
		Method:      rr.Method,
		URL:         rr.URL,
		StatusCode:  rr.StatusCode,
		Request:     rr.Request,
		Response:    rr.Response,
		CreatedOn:   time.Now(),
		Elapsed:     rr.Elapsed,
	}

	return log
}

// NewChannelLogFromError creates a new channel log for the passed in channel, msg id and error
func NewChannelLogFromError(description string, channel Channel, msgID MsgID, elapsed time.Duration, err error) *ChannelLog {
	log := &ChannelLog{
		Description: description,
		Channel:     channel,
		MsgID:       msgID,
		Error:       err.Error(),
		CreatedOn:   time.Now(),
		Elapsed:     elapsed,
	}

	return log
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
