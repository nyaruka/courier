package models

import (
	"database/sql/driver"
	"strconv"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// ChannelEventID is the type of our channel event ids
type ChannelEventID null.Int

// ChannelEvent is our typing of a channelevent's UUID
type ChannelEventUUID uuids.UUID

// ChannelEventType is the type of channel event this is
type ChannelEventType string

// Possible values for ChannelEventTypes
const (
	EventTypeNewConversation ChannelEventType = "new_conversation"
	EventTypeReferral        ChannelEventType = "referral"
	EventTypeStopContact     ChannelEventType = "stop_contact"
	EventTypeWelcomeMessage  ChannelEventType = "welcome_message"
	EventTypeOptIn           ChannelEventType = "optin"
	EventTypeOptOut          ChannelEventType = "optout"
)

const NilChannelEventID = ChannelEventID(0)

func (i *ChannelEventID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ChannelEventID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ChannelEventID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ChannelEventID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// String satisfies the Stringer interface
func (i ChannelEventID) String() string {
	if i != NilChannelEventID {
		return strconv.FormatInt(int64(i), 10)
	}
	return "null"
}
