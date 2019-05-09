package courier

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/nyaruka/null"

	"github.com/gofrs/uuid"
	"github.com/nyaruka/gocommon/urns"
)

// ErrMsgNotFound is returned when trying to queue the status for a Msg that doesn't exit
var ErrMsgNotFound = errors.New("message not found")

// ErrWrongIncomingMsgStatus use do ignore the status update if the DB raise this
var ErrWrongIncomingMsgStatus = errors.New("Incoming messages can only be PENDING or HANDLED")

// MsgID is our typing of the db int type
type MsgID null.Int

// NewMsgID creates a new MsgID for the passed in int64
func NewMsgID(id int64) MsgID {
	return MsgID(id)
}

// String satisfies the Stringer interface
func (i MsgID) String() string {
	if i != NilMsgID {
		return strconv.FormatInt(int64(i), 10)
	}
	return "null"
}

// MarshalJSON marshals into JSON. 0 values will become null
func (i MsgID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

// UnmarshalJSON unmarshals from JSON. null values become 0
func (i *MsgID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

// Value returns the db value, null is returned for 0
func (i MsgID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

// Scan scans from the db value. null values become 0
func (i *MsgID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID(0)

// MsgUUID is the UUID of a message which has been received
type MsgUUID struct {
	uuid.UUID
}

// NilMsgUUID is a "zero value" message UUID
var NilMsgUUID = MsgUUID{uuid.Nil}

// NewMsgUUID creates a new unique message UUID
func NewMsgUUID() MsgUUID {
	u, _ := uuid.NewV4()
	return MsgUUID{u}
}

// NewMsgUUIDFromString creates a new message UUID for the passed in string
func NewMsgUUIDFromString(uuidString string) MsgUUID {
	uuid, _ := uuid.FromString(uuidString)
	return MsgUUID{uuid}
}

//-----------------------------------------------------------------------------
// Msg interface
//-----------------------------------------------------------------------------

// Msg is our interface to represent an incoming or outgoing message
type Msg interface {
	ID() MsgID
	UUID() MsgUUID
	Text() string
	Attachments() []string
	ExternalID() string
	URN() urns.URN
	URNAuth() string
	ContactName() string
	QuickReplies() []string
	Metadata() json.RawMessage
	ResponseToID() MsgID
	ResponseToExternalID() string

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
	WithURNAuth(auth string) Msg
	WithMetadata(metadata json.RawMessage) Msg

	EventID() int64
}
