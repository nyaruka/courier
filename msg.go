package courier

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
)

// ErrMsgNotFound is returned when trying to queue the status for a Msg that doesn't exit
var ErrMsgNotFound = errors.New("message not found")

// MsgID is our typing of the db int type
type MsgID struct {
	sql.NullInt64
}

// NewMsgID creates a new MsgID for the passed in int64
func NewMsgID(id int64) MsgID {
	return MsgID{sql.NullInt64{Int64: id, Valid: true}}
}

// UnmarshalText satisfies text unmarshalling so ids can be decoded from forms
func (i *MsgID) UnmarshalText(text []byte) (err error) {
	id, err := strconv.Atoi(string(text))
	i.Int64 = int64(id)
	if err != nil {
		i.Valid = false
	}
	return err
}

// UnmarshalJSON satisfies json unmarshalling so ids can be decoded from JSON
func (i *MsgID) UnmarshalJSON(bytes []byte) (err error) {
	return json.Unmarshal(bytes, &i.NullInt64)
}

// String satisfies the Stringer interface
func (i *MsgID) String() string {
	return fmt.Sprintf("%d", i.Int64)
}

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID{sql.NullInt64{Int64: 0, Valid: false}}

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
	URN() URN
	ContactName() string

	ReceivedOn() *time.Time
	SentOn() *time.Time

	WithContactName(name string) Msg
	WithReceivedOn(date time.Time) Msg
	WithExternalID(id string) Msg
	WithID(id MsgID) Msg
	WithUUID(uuid MsgUUID) Msg
	WithAttachment(url string) Msg
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
