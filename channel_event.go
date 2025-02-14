package courier

import (
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

// ChannelEventType is the type of channel event this is
type ChannelEventType string

// ChannelEvent is our typing of a channelevent's UUID
type ChannelEventUUID uuids.UUID

// Possible values for ChannelEventTypes
const (
	EventTypeNewConversation ChannelEventType = "new_conversation"
	EventTypeReferral        ChannelEventType = "referral"
	EventTypeStopContact     ChannelEventType = "stop_contact"
	EventTypeWelcomeMessage  ChannelEventType = "welcome_message"
	EventTypeOptIn           ChannelEventType = "optin"
	EventTypeOptOut          ChannelEventType = "optout"
)

//-----------------------------------------------------------------------------
// ChannelEvent Interface
//-----------------------------------------------------------------------------

// ChannelEvent represents an event on a channel, such as a follow, new conversation or referral
type ChannelEvent interface {
	Event

	UUID() ChannelEventUUID
	ChannelUUID() ChannelUUID
	URN() urns.URN
	EventType() ChannelEventType
	Extra() map[string]string
	CreatedOn() time.Time
	OccurredOn() time.Time

	WithContactName(name string) ChannelEvent
	WithURNAuthTokens(tokens map[string]string) ChannelEvent
	WithExtra(extra map[string]string) ChannelEvent
	WithOccurredOn(time.Time) ChannelEvent
}
