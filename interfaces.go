package courier

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

// Event is our interface for the types of things a ChannelHandleFunc can return.
type Event interface {
	EventUUID() uuids.UUID
}

// Msg is our interface for common methods for an incoming or outgoing message
type Msg interface {
	UUID() models.MsgUUID
	Text() string
	Attachments() []string
	URN() urns.URN
	Channel() *models.Channel
}

// MsgOut is our interface to represent an outgoing
type MsgOut interface {
	Msg

	Contact() *models.ContactReference
	QuickReplies() []models.QuickReply
	Locale() i18n.Locale
	Templating() *models.Templating
	URNAuth() string
	Origin() models.MsgOrigin
	ResponseToExternalID() string
	IsResend() bool
	Flow() *models.FlowReference
	UserID() models.UserID
	HighPriority() bool
	Session() *models.Session
}

// MsgIn is our interface to represent an incoming
type MsgIn interface {
	Event
	Msg

	ExternalID() string
	ReceivedOn() *time.Time
	WithAttachment(url string) MsgIn
	WithContactName(name string) MsgIn
	WithURNAuthTokens(tokens map[string]string) MsgIn
	WithReceivedOn(date time.Time) MsgIn
	WithNewURN(urn urns.URN, action models.NewURNAction) MsgIn
	WithPayload(payload json.RawMessage) MsgIn
}

// ChannelEvent represents an event on a channel, such as a follow, new conversation or referral
type ChannelEvent interface {
	Event

	UUID() models.ChannelEventUUID
	ChannelUUID() models.ChannelUUID
	URN() urns.URN
	EventType() models.ChannelEventType
	Extra() map[string]string
	OccurredOn() time.Time

	WithContactName(name string) ChannelEvent
	WithExtra(extra map[string]string) ChannelEvent
	WithOccurredOn(time.Time) ChannelEvent
}
