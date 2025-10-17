package models

import (
	"database/sql/driver"
	"strconv"
	"time"

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
