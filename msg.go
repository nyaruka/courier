package courier

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v2"
)

// ErrMsgNotFound is returned when trying to queue the status for a Msg that doesn't exit
var ErrMsgNotFound = errors.New("message not found")

// ErrWrongIncomingMsgStatus use do ignore the status update if the DB raise this
var ErrWrongIncomingMsgStatus = errors.New("incoming messages can only be PENDING or HANDLED")

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

// Msg is our interface to represent an incoming or outgoing message
type Msg interface {
	ID() MsgID
	UUID() MsgUUID
	Text() string
	Attachments() []string
	Locale() utils.Locale
	ExternalID() string
	URN() urns.URN
	URNAuth() string
	ContactName() string
	QuickReplies() []string
	Origin() MsgOrigin
	ContactLastSeenOn() *time.Time
	Topic() string
	Metadata() json.RawMessage
	ResponseToExternalID() string
	IsResend() bool

	Flow() *FlowReference
	FlowName() string
	FlowUUID() string

	Channel() Channel

	ReceivedOn() *time.Time
	SentOn() *time.Time

	HighPriority() bool

	WithContactName(name string) Msg
	WithReceivedOn(date time.Time) Msg
	WithExternalID(id string) Msg
	WithID(id MsgID) Msg
	WithUUID(uuid MsgUUID) Msg
	WithAttachment(url string) Msg
	WithLocale(utils.Locale) Msg
	WithURNAuth(auth string) Msg
	WithMetadata(metadata json.RawMessage) Msg
	WithFlow(flow *FlowReference) Msg

	EventID() int64
	SessionStatus() string
}
