package models

import (
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

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

// MsgIn is an incoming message which can be written to the database or marshaled to a spool file
type MsgIn struct {
	OrgID_              OrgID          `db:"org_id"                 json:"org_id"`
	UUID_               MsgUUID        `db:"uuid"                   json:"uuid"`
	Text_               string         `db:"text"                   json:"text"`
	Attachments_        pq.StringArray `db:"attachments"            json:"attachments"`
	ExternalIdentifier_ null.String    `db:"external_identifier"    json:"external_identifier"`
	ChannelID_          ChannelID      `db:"channel_id"             json:"channel_id"`
	ContactID_          ContactID      `db:"contact_id"             json:"contact_id"`
	ContactURNID_       ContactURNID   `db:"contact_urn_id"         json:"contact_urn_id"`
	CreatedOn_          time.Time      `db:"created_on"             json:"created_on"`
	ModifiedOn_         time.Time      `db:"modified_on"            json:"modified_on"`
	SentOn_             *time.Time     `db:"sent_on"                json:"sent_on"`
	LogUUIDs            pq.StringArray `db:"log_uuids"              json:"log_uuids"`
}

// NewIncomingMsg creates a new incoming message
func NewIncomingMsg(channel *Channel, urn urns.URN, text string, extID string, clogUUID clogs.UUID) *MsgIn {
	now := time.Now()

	return &MsgIn{
		OrgID_:              channel.OrgID(),
		UUID_:               MsgUUID(uuids.NewV7()),
		Text_:               text,
		ExternalIdentifier_: null.String(extID),
		ChannelID_:          channel.ID(),
		CreatedOn_:          now,
		ModifiedOn_:         now,
		SentOn_:             &now,
		LogUUIDs:            pq.StringArray{string(clogUUID)},
	}
}

func (m *MsgIn) EventUUID() uuids.UUID  { return uuids.UUID(m.UUID_) }
func (m *MsgIn) UUID() MsgUUID          { return m.UUID_ }
func (m *MsgIn) ExternalID() string     { return string(m.ExternalIdentifier_) }
func (m *MsgIn) Text() string           { return m.Text_ }
func (m *MsgIn) Attachments() []string  { return []string(m.Attachments_) }
func (m *MsgIn) ReceivedOn() *time.Time { return m.SentOn_ }

type MsgOrigin string

const (
	MsgOriginFlow      MsgOrigin = "flow"
	MsgOriginBroadcast MsgOrigin = "broadcast"
	MsgOriginTicket    MsgOrigin = "ticket"
	MsgOriginChat      MsgOrigin = "chat"
)

const QuickReplyTypeLocation = "location"

type QuickReply struct {
	Type  string `json:"type"            validate:"required"`
	Text  string `json:"text,omitempty"`
	Extra string `json:"extra,omitempty"`
}

func (qr QuickReply) GetText() string {
	if qr.Type == QuickReplyTypeLocation && qr.Text == "" {
		return "Send Location"
	}
	return qr.Text
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
		UUID string `json:"uuid" validate:"uuid,required"`
		Name string `json:"name" validate:"required"`
	} `json:"template" validate:"required"`
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
}

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

// QuickRepliesToRows takes a slice of quick replies and re-organizes it into rows and columns
func QuickRepliesToRows(replies []QuickReply, maxRows, maxRowRunes, paddingRunes int) [][]QuickReply {
	// calculate rune length if it's all one row
	totalRunes := 0
	for i := range replies {
		totalRunes += utf8.RuneCountInString(replies[i].Text) + paddingRunes*2
	}

	if totalRunes <= maxRowRunes {
		// if all strings fit on a single row, do that
		return [][]QuickReply{replies}
	} else if len(replies) <= maxRows {
		// if each string can be a row, do that
		rows := make([][]QuickReply, len(replies))
		for i := range replies {
			rows[i] = []QuickReply{replies[i]}
		}
		return rows
	}

	rows := [][]QuickReply{{}}
	curRow := 0
	rowRunes := 0

	for _, reply := range replies {
		strRunes := utf8.RuneCountInString(reply.Text) + paddingRunes*2

		// take a new row if we can't fit this string and the current row isn't empty and we haven't hit the row limit
		if rowRunes+strRunes > maxRowRunes && len(rows[curRow]) > 0 && len(rows) < maxRows {
			rows = append(rows, []QuickReply{})
			curRow += 1
			rowRunes = 0
		}

		rows[curRow] = append(rows[curRow], reply)
		rowRunes += strRunes
	}
	return rows
}
