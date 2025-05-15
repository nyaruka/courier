package courier

import (
	"database/sql/driver"
	"strconv"
	"time"

	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// MsgID is our typing of the db int type
type MsgID null.Int64

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID(0)

// String satisfies the Stringer interface
func (i MsgID) String() string { return strconv.FormatInt(int64(i), 10) }

func (i *MsgID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i MsgID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *MsgID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i MsgID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// MsgUUID is the UUID of a message which has been received
type MsgUUID uuids.UUID

// NilMsgUUID is a "zero value" message UUID
const NilMsgUUID = MsgUUID("")

type QuickReply struct {
	Text  string `json:"text"`
	Extra string `json:"extra,omitempty"`
}

func (q *QuickReply) UnmarshalJSON(d []byte) error {
	// if we just have a string we unmarshal it into the text field
	if len(d) > 2 && d[0] == '"' && d[len(d)-1] == '"' {
		return jsonx.Unmarshal(d, &q.Text)
	}

	// alias our type so we don't end up here again
	type alias QuickReply

	return jsonx.Unmarshal(d, (*alias)(q))
}

type FlowReference struct {
	UUID string `json:"uuid" validate:"uuid4"`
	Name string `json:"name"`
}

type OptInReference struct {
	ID   int64  `json:"id"   validate:"required"`
	Name string `json:"name" validate:"required"`
}

type UserID int

type MsgOrigin string

const (
	MsgOriginFlow      MsgOrigin = "flow"
	MsgOriginBroadcast MsgOrigin = "broadcast"
	MsgOriginTicket    MsgOrigin = "ticket"
	MsgOriginChat      MsgOrigin = "chat"
)

type TemplatingVariable struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Templating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace  string `json:"namespace"`
	Components []struct {
		Type      string         `json:"type"`
		Name      string         `json:"name"`
		Variables map[string]int `json:"variables"`
	} `json:"components"`
	Variables  []TemplatingVariable `json:"variables"`
	Language   string               `json:"language"`
	ExternalID string               `json:"external_id"`
}

type Session struct {
	UUID       string `json:"uuid"`
	Status     string `json:"status"`
	SprintUUID string `json:"sprint_uuid"`
	Timeout    int    `json:"timeout"`
}

//-----------------------------------------------------------------------------
// Msg interface
//-----------------------------------------------------------------------------

// Msg is our interface for common methods for an incoming or outgoing message
type Msg interface {
	Event

	ID() MsgID
	UUID() MsgUUID
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
	QuickReplies() []QuickReply
	Locale() i18n.Locale
	Templating() *Templating
	URNAuth() string
	Origin() MsgOrigin
	ContactLastSeenOn() *time.Time
	ResponseToExternalID() string
	SentOn() *time.Time
	IsResend() bool
	Flow() *FlowReference
	OptIn() *OptInReference
	UserID() UserID
	HighPriority() bool
	Session() *Session
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
