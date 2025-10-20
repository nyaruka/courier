package models

import (
	"database/sql/driver"
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

// MsgStatus is the status of a message
type MsgStatus string

// Possible values for MsgStatus
const (
	MsgStatusPending   MsgStatus = "P"
	MsgStatusQueued    MsgStatus = "Q"
	MsgStatusWired     MsgStatus = "W"
	MsgStatusSent      MsgStatus = "S"
	MsgStatusDelivered MsgStatus = "D"
	MsgStatusRead      MsgStatus = "R"
	MsgStatusErrored   MsgStatus = "E"
	MsgStatusFailed    MsgStatus = "F"
)

// MsgDirection is the direction of a message
type MsgDirection string

// Possible values for MsgDirection
const (
	MsgIncoming MsgDirection = "I"
	MsgOutgoing MsgDirection = "O"
)

// MsgVisibility is the visibility of a message
type MsgVisibility string

// Possible values for MsgVisibility
const (
	MsgVisible  MsgVisibility = "V"
	MsgDeleted  MsgVisibility = "D"
	MsgArchived MsgVisibility = "A"
)

type MsgOrigin string

const (
	MsgOriginFlow      MsgOrigin = "flow"
	MsgOriginBroadcast MsgOrigin = "broadcast"
	MsgOriginTicket    MsgOrigin = "ticket"
	MsgOriginChat      MsgOrigin = "chat"
)

type QuickReply struct {
	Text  string `json:"text"            validate:"required"`
	Extra string `json:"extra,omitempty"`
}

// ContactReference is information about a contact provided on queued outgoing messages
type ContactReference struct {
	ID         ContactID   `json:"id"   validate:"required"`      // for creating session timeout fires in Postgres
	UUID       ContactUUID `json:"uuid" validate:"uuid,required"` // for creating status updates in DynamoDB
	LastSeenOn *time.Time  `json:"last_seen_on,omitempty"`
}

// FlowReference is a reference to a flow on a queued outgoing message
type FlowReference struct {
	UUID string `json:"uuid" validate:"uuid"`
	Name string `json:"name"`
}

type OptInReference struct {
	ID   int64  `json:"id"   validate:"required"`
	Name string `json:"name" validate:"required"`
}

type TemplatingVariable struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Templating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"uuid,required"`
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
	UUID       string `json:"uuid"        validate:"uuid,required"`
	Status     string `json:"status"`
	SprintUUID string `json:"sprint_uuid" validate:"omitempty,uuid"`
	Timeout    int    `json:"timeout"`
}

type MsgOut struct {
	OrgID_                OrgID             `json:"org_id"         validate:"required"`
	UUID_                 MsgUUID           `json:"uuid"           validate:"required"`
	Contact_              *ContactReference `json:"contact"        validate:"required"`
	HighPriority_         bool              `json:"high_priority"`
	Text_                 string            `json:"text"`
	Attachments_          []string          `json:"attachments"`
	QuickReplies_         []QuickReply      `json:"quick_replies"`
	Locale_               i18n.Locale       `json:"locale"`
	Templating_           *Templating       `json:"templating"`
	CreatedOn_            time.Time         `json:"created_on"     validate:"required"`
	ChannelUUID_          ChannelUUID       `json:"channel_uuid"   validate:"required"`
	URN_                  urns.URN          `json:"urn"            validate:"required"`
	URNAuth_              string            `json:"urn_auth"`
	ResponseToExternalID_ string            `json:"response_to_external_id"`
	IsResend_             bool              `json:"is_resend"`
	Flow_                 *FlowReference    `json:"flow"`
	OptIn_                *OptInReference   `json:"optin"`
	UserID_               UserID            `json:"user_id"`
	Origin_               MsgOrigin         `json:"origin"         validate:"required"`
	Session_              *Session          `json:"session"`

	// deprecated: need to rework some handlers to not use this for status callbacks
	ID_ MsgID `json:"id"             validate:"required"`
}

func (m *MsgOut) EventUUID() uuids.UUID        { return uuids.UUID(m.UUID_) }
func (m *MsgOut) ID() MsgID                    { return m.ID_ }
func (m *MsgOut) UUID() MsgUUID                { return m.UUID_ }
func (m *MsgOut) Contact() *ContactReference   { return m.Contact_ }
func (m *MsgOut) Text() string                 { return m.Text_ }
func (m *MsgOut) Attachments() []string        { return m.Attachments_ }
func (m *MsgOut) URN() urns.URN                { return m.URN_ }
func (m *MsgOut) QuickReplies() []QuickReply   { return m.QuickReplies_ }
func (m *MsgOut) Locale() i18n.Locale          { return m.Locale_ }
func (m *MsgOut) Templating() *Templating      { return m.Templating_ }
func (m *MsgOut) URNAuth() string              { return m.URNAuth_ }
func (m *MsgOut) Origin() MsgOrigin            { return m.Origin_ }
func (m *MsgOut) ResponseToExternalID() string { return m.ResponseToExternalID_ }
func (m *MsgOut) IsResend() bool               { return m.IsResend_ }
func (m *MsgOut) Flow() *FlowReference         { return m.Flow_ }
func (m *MsgOut) OptIn() *OptInReference       { return m.OptIn_ }
func (m *MsgOut) UserID() UserID               { return m.UserID_ }
func (m *MsgOut) Session() *Session            { return m.Session_ }
func (m *MsgOut) HighPriority() bool           { return m.HighPriority_ }
