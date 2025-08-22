package courier

import (
	"time"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/urns"
)

//-----------------------------------------------------------------------------
// ChannelEvent Interface
//-----------------------------------------------------------------------------

// ChannelEvent represents an event on a channel, such as a follow, new conversation or referral
type ChannelEvent interface {
	Event

	UUID() models.ChannelEventUUID
	ChannelUUID() models.ChannelUUID
	URN() urns.URN
	EventType() models.ChannelEventType
	Extra() map[string]string
	CreatedOn() time.Time
	OccurredOn() time.Time

	WithContactName(name string) ChannelEvent
	WithURNAuthTokens(tokens map[string]string) ChannelEvent
	WithExtra(extra map[string]string) ChannelEvent
	WithOccurredOn(time.Time) ChannelEvent
}
