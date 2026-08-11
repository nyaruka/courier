package courier

import (
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
)

// The constructors below just name the log type for their caller. They should fold into models.NewChannelLog once
// MsgOut is concrete too and NewChannelLogForSend can live alongside it.

// NewChannelLogForIncoming creates a new channel log for an incoming request, the type of which won't be known
// until the handler completes.
func NewChannelLogForIncoming(logType clogs.Type, ch *models.Channel, r *httpx.Recorder, redactVals []string) *models.ChannelLog {
	return models.NewChannelLog(logType, ch, r, redactVals)
}

// NewChannelLogForSend creates a new channel log for a message send
func NewChannelLogForSend(msg MsgOut, redactVals []string) *models.ChannelLog {
	return models.NewChannelLog(models.ChannelLogTypeMsgSend, msg.Channel(), nil, redactVals)
}

// NewChannelLogForAttachmentFetch creates a new channel log for an attachment fetch
func NewChannelLogForAttachmentFetch(ch *models.Channel, redactVals []string) *models.ChannelLog {
	return models.NewChannelLog(models.ChannelLogTypeAttachmentFetch, ch, nil, redactVals)
}

// NewChannelLogForEventSend creates a new channel log for an event send
func NewChannelLogForEventSend(ch *models.Channel, redactVals []string) *models.ChannelLog {
	return models.NewChannelLog(models.ChannelLogTypeEventSend, ch, nil, redactVals)
}

// NewChannelLog creates a new channel log with the given type and channel
func NewChannelLog(t clogs.Type, ch *models.Channel, redactVals []string) *models.ChannelLog {
	return models.NewChannelLog(t, ch, nil, redactVals)
}
