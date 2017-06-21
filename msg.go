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

	"github.com/nyaruka/courier/queue"
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

// NewIncomingMsg creates a new message from the given params
func NewIncomingMsg(channel Channel, urn URN, text string) *Msg {
	m := &Msg{}
	m.UUID = NewMsgUUID()
	m.Channel = channel
	m.Text = text
	m.URN = urn

	now := time.Now()
	m.ReceivedOn = &now

	return m
}

// NewOutgoingMsg creates a new message from the given params
func NewOutgoingMsg(channel Channel, urn URN, text string) *Msg {
	m := &Msg{}
	m.UUID = NewMsgUUID()
	m.Channel = channel
	m.Text = text
	m.URN = urn

	return m
}

//-----------------------------------------------------------------------------
// Msg implementation
//-----------------------------------------------------------------------------

// Msg is our base struct to represent an incoming or outgoing message
type Msg struct {
	Channel     Channel
	ID          MsgID
	UUID        MsgUUID
	Text        string
	Attachments []string
	ExternalID  string
	URN         URN
	ContactName string

	WorkerToken queue.WorkerToken

	ReceivedOn *time.Time
	SentOn     *time.Time
	WiredOn    *time.Time
}

// WithContactName can be used to set the contact name on a msg
func (m *Msg) WithContactName(name string) *Msg { m.ContactName = name; return m }

// WithReceivedOn can be used to set sent_on on a msg in a chained call
func (m *Msg) WithReceivedOn(date time.Time) *Msg { m.ReceivedOn = &date; return m }

// WithExternalID can be used to set the external id on a msg in a chained call
func (m *Msg) WithExternalID(id string) *Msg { m.ExternalID = id; return m }

// AddAttachment can be used to append to the media urls for a message
func (m *Msg) AddAttachment(url string) *Msg { m.Attachments = append(m.Attachments, url); return m }

// TextAndAttachments returns both the text of our message as well as any attachments, newline delimited
func (m *Msg) TextAndAttachments() string {
	buf := bytes.NewBuffer([]byte(m.Text))
	for _, a := range m.Attachments {
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
