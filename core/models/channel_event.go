package models

import "github.com/nyaruka/gocommon/uuids"

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
