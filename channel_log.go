package courier

import (
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
)

// The constructors below exist because callers here hold a Channel interface rather than a *models.Channel. They
// should collapse into models.NewChannelLog once that interface is gone.

// NewChannelLogForIncoming creates a new channel log for an incoming request, the type of which won't be known
// until the handler completes.
func NewChannelLogForIncoming(logType clogs.Type, ch Channel, r *httpx.Recorder, redactVals []string) *models.ChannelLog {
	return newChannelLog(logType, ch, r, redactVals)
}

// NewChannelLogForSend creates a new channel log for a message send
func NewChannelLogForSend(msg MsgOut, redactVals []string) *models.ChannelLog {
	return newChannelLog(models.ChannelLogTypeMsgSend, msg.Channel(), nil, redactVals)
}

// NewChannelLogForAttachmentFetch creates a new channel log for an attachment fetch
func NewChannelLogForAttachmentFetch(ch Channel, redactVals []string) *models.ChannelLog {
	return newChannelLog(models.ChannelLogTypeAttachmentFetch, ch, nil, redactVals)
}

// NewChannelLogForEventSend creates a new channel log for an event send
func NewChannelLogForEventSend(ch Channel, redactVals []string) *models.ChannelLog {
	return newChannelLog(models.ChannelLogTypeEventSend, ch, nil, redactVals)
}

// NewChannelLog creates a new channel log with the given type and channel
func NewChannelLog(t clogs.Type, ch Channel, redactVals []string) *models.ChannelLog {
	return newChannelLog(t, ch, nil, redactVals)
}

func newChannelLog(t clogs.Type, ch Channel, r *httpx.Recorder, redactVals []string) *models.ChannelLog {
	// channel can be nil, e.g. for webhook verification requests which aren't for any particular channel
	if ch == nil {
		return models.NewChannelLog(t, models.NilChannelUUID, 0, r, redactVals)
	}
	return models.NewChannelLog(t, ch.UUID(), ch.OrgID(), r, redactVals)
}
