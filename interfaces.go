package courier

import (
	"time"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

// Channel defines the general interface backend Channel implementations must adhere to
type Channel interface {
	UUID() models.ChannelUUID
	Name() string
	ChannelType() models.ChannelType
	Schemes() []string
	Country() i18n.Country
	Address() string
	ChannelAddress() models.ChannelAddress

	Roles() []models.ChannelRole

	// is this channel for the passed in scheme (and only that scheme)
	IsScheme(*urns.Scheme) bool

	// CallbackDomain returns the domain that should be used for any callbacks the channel registers
	CallbackDomain(fallbackDomain string) string

	ConfigForKey(key string, defaultValue any) any
	StringConfigForKey(key string, defaultValue string) string
	BoolConfigForKey(key string, defaultValue bool) bool
	IntConfigForKey(key string, defaultValue int) int
	OrgConfigForKey(key string, defaultValue any) any
}

// Contact defines the attributes on a contact, for our purposes that is just a contact UUID
type Contact interface {
	UUID() models.ContactUUID
}

// Event is our interface for the types of things a ChannelHandleFunc can return.
type Event interface {
	EventUUID() uuids.UUID
}

// Msg is our interface for common methods for an incoming or outgoing message
type Msg interface {
	Event

	ID() models.MsgID
	UUID() models.MsgUUID
	ExternalID() string
	Text() string
	Attachments() []string
	URN() urns.URN
	Channel() Channel
}

// MsgOut is our interface to represent an outgoing
type MsgOut interface {
	Msg

	// outgoing specific
	QuickReplies() []models.QuickReply
	Locale() i18n.Locale
	Templating() *models.Templating
	URNAuth() string
	Origin() models.MsgOrigin
	ContactLastSeenOn() *time.Time
	ResponseToExternalID() string
	SentOn() *time.Time
	IsResend() bool
	Flow() *models.FlowReference
	OptIn() *models.OptInReference
	UserID() models.UserID
	HighPriority() bool
	Session() *models.Session
}

// MsgIn is our interface to represent an incoming
type MsgIn interface {
	Msg

	// incoming specific
	ReceivedOn() *time.Time
	WithAttachment(url string) MsgIn
	WithContactName(name string) MsgIn
	WithURNAuthTokens(tokens map[string]string) MsgIn
	WithReceivedOn(date time.Time) MsgIn
}

// StatusUpdate represents a status update on a message
type StatusUpdate interface {
	Event

	ChannelUUID() models.ChannelUUID
	MsgUUID() models.MsgUUID
	MsgID() models.MsgID

	SetURNUpdate(old, new urns.URN) error
	URNUpdate() (old, new urns.URN)

	ExternalID() string
	SetExternalID(string)

	Status() models.MsgStatus
	SetStatus(models.MsgStatus)
}

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
