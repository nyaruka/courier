package courier

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	null "gopkg.in/guregu/null.v3"

	"github.com/nyaruka/gocommon/urns"
	uuid "github.com/satori/go.uuid"
)

// ErrMsgNotFound is returned when trying to queue the status for a Msg that doesn't exit
var ErrMsgNotFound = errors.New("message not found")

// MsgID is our typing of the db int type
type MsgID struct {
	null.Int
}

// NewMsgID creates a new MsgID for the passed in int64
func NewMsgID(id int64) MsgID {
	return MsgID{null.NewInt(id, true)}
}

// String satisfies the Stringer interface
func (i MsgID) String() string {
	if i.Valid {
		return strconv.FormatInt(i.Int64, 10)
	}
	return "null"
}

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID{null.NewInt(0, false)}

// MsgUUID is the UUID of a message which has been received
type MsgUUID struct {
	uuid.UUID
}

// NilMsgUUID is a "zero value" message UUID
var NilMsgUUID = MsgUUID{uuid.Nil}

// NewMsgUUID creates a new unique message UUID
func NewMsgUUID() MsgUUID {
	return MsgUUID{uuid.NewV4()}
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
	Channel() Channel
	ID() MsgID
	UUID() MsgUUID
	Text() string
	Attachments() []string
	ExternalID() string
	URN() urns.URN
	ContactName() string
	QuickReplies() []string

	ReceivedOn() *time.Time
	SentOn() *time.Time

	HighPriority() bool

	WithContactName(name string) Msg
	WithReceivedOn(date time.Time) Msg
	WithExternalID(id string) Msg
	WithID(id MsgID) Msg
	WithUUID(uuid MsgUUID) Msg
	WithAttachment(url string) Msg

	ReceiveID() int64
}

// GetTextAndAttachments returns both the text of our message as well as any attachments, newline delimited
func GetTextAndAttachments(m Msg) string {
	buf := bytes.NewBuffer([]byte(m.Text()))
	for _, a := range m.Attachments() {
		_, url := SplitAttachment(a)
		buf.WriteString("\n")
		buf.WriteString(url)
	}
	return buf.String()
}

// SplitAttachment takes an attachment string and returns the media type and URL for the attachment
func SplitAttachment(attachment string) (string, string) {
	parts := strings.SplitN(attachment, ":", 2)
	if len(parts) < 2 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}
