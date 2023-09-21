package courier

import (
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"

	"github.com/nyaruka/gocommon/i18n"
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

type FlowReference struct {
	UUID string `json:"uuid" validate:"uuid4"`
	Name string `json:"name"`
}

type OptInReference struct {
	ID   int64  `json:"id"   validate:"required"`
	Name string `json:"name" validate:"required"`
}

type MsgOrigin string

const (
	MsgOriginFlow      MsgOrigin = "flow"
	MsgOriginBroadcast MsgOrigin = "broadcast"
	MsgOriginTicket    MsgOrigin = "ticket"
	MsgOriginChat      MsgOrigin = "chat"
)

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
	QuickReplies() []string
	Locale() i18n.Locale
	URNAuth() string
	Origin() MsgOrigin
	ContactLastSeenOn() *time.Time
	Topic() string
	Metadata() json.RawMessage
	ResponseToExternalID() string
	SentOn() *time.Time
	IsResend() bool
	Flow() *FlowReference
	OptIn() *OptInReference
	SessionStatus() string
	HighPriority() bool
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
