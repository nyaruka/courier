package courier

import (
	"time"

	"github.com/nyaruka/gocommon/urns"
)

// ChannelEventType is the type of channel event this is
type ChannelEventType string

// Possible values for ChannelEventTypes
const (
	NewConversation ChannelEventType = "new_conversation"
	Referral        ChannelEventType = "referral"
	StopContact     ChannelEventType = "stop_contact"
	WelcomeMessage  ChannelEventType = "welcome_message"
)

//-----------------------------------------------------------------------------
// ChannelEvent Interface
//-----------------------------------------------------------------------------

// ChannelEvent represents an event on a channel, such as a follow, new conversation or referral
type ChannelEvent interface {
	ChannelUUID() ChannelUUID
	URN() urns.URN
	EventType() ChannelEventType
	Extra() map[string]interface{}
	CreatedOn() time.Time
	OccurredOn() time.Time

	Logs() []*ChannelLog
	AddLog(log *ChannelLog)

	WithContactName(name string) ChannelEvent
	WithExtra(extra map[string]interface{}) ChannelEvent
	WithOccurredOn(time.Time) ChannelEvent

	EventID() int64
}
