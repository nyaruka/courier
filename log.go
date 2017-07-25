package courier

import (
	"fmt"
	"time"

	"github.com/nyaruka/courier/utils"
)

// NilStatusCode is used when we have an error before even sending anything
const NilStatusCode int = 417

// NewChannelLog creates a new channel log for the passed in channel, id, and request and response info
func NewChannelLog(channel Channel, msgID MsgID, method string, url string, statusCode int, err error,
	request string, response string, elapsed time.Duration, createdOn time.Time) *ChannelLog {

	errString := ""
	if err != nil {
		errString = err.Error()
	}

	return &ChannelLog{
		Channel:    channel,
		MsgID:      msgID,
		Method:     method,
		URL:        url,
		StatusCode: statusCode,
		Error:      errString,
		Request:    request,
		Response:   response,
		CreatedOn:  createdOn,
		Elapsed:    elapsed,
	}
}

// NewChannelLogFromRR creates a new channel log for the passed in channel, id, and request/response log
func NewChannelLogFromRR(channel Channel, msgID MsgID, rr *utils.RequestResponse) *ChannelLog {
	return &ChannelLog{
		Channel:    channel,
		MsgID:      msgID,
		Method:     rr.Method,
		URL:        rr.URL,
		StatusCode: rr.StatusCode,
		Error:      "",
		Request:    rr.Request,
		Response:   rr.Response,
		CreatedOn:  time.Now(),
		Elapsed:    rr.Elapsed,
	}
}

func (l *ChannelLog) String() string {
	return fmt.Sprintf("%d %s %d\n%s\n%s\n%s", l.StatusCode, l.URL, l.Elapsed, l.Error, l.Request, l.Response)
}

// ChannelLog represents the log for a msg being received, sent or having its status updated. It includes the HTTP request
// and response for the action as well as the channel it was performed on and an option ID of the msg (for some error
// cases we may log without a msg id)
type ChannelLog struct {
	Channel    Channel
	MsgID      MsgID
	Method     string
	URL        string
	StatusCode int
	Error      string
	Request    string
	Response   string
	Elapsed    time.Duration
	CreatedOn  time.Time
}
