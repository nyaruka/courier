package courier

import (
	"time"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
)

//-----------------------------------------------------------------------------
// Msg interface
//-----------------------------------------------------------------------------

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
