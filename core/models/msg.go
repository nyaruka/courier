package models

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
	"github.com/vinovest/sqlx"
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

// NewURNAction is the action to perform with a new URN
type NewURNAction string

const (
	NewURNAppend NewURNAction = "append"
)

// NewURNSpec specifies a new URN to add or replace on the contact
type NewURNSpec struct {
	Value  urns.URN     `json:"value"`
	Action NewURNAction `json:"action"`
}

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

// MsgIn is an incoming message as created by a handler, and is also what we marshal to spool files when the
// database is down. It doesn't carry any database ids - those are resolved when it's written.
type MsgIn struct {
	UUID_        MsgUUID      `json:"uuid"`
	ChannelUUID_ ChannelUUID  `json:"channel_uuid"`
	URN_         urns.URN     `json:"urn"`
	Text_        string       `json:"text"`
	Attachments_ []string     `json:"attachments,omitempty"`
	ExternalID_  string       `json:"external_id,omitempty"`
	CreatedOn_   time.Time    `json:"created_on"`
	ReceivedOn_  *time.Time   `json:"received_on,omitempty"`
	LogUUIDs     []clogs.UUID `json:"log_uuids"`

	// optional extras set by handlers, used to update the contact or passed to mailroom for handling
	ContactName_   string            `json:"contact_name,omitempty"`
	URNAuthTokens_ map[string]string `json:"auth_tokens,omitempty"`
	NewURN_        *NewURNSpec       `json:"new_urn,omitempty"`
	Payload_       json.RawMessage   `json:"payload,omitempty"`

	Channel_   *Channel `json:"-"`
	Duplicate_ bool     `json:"-"`
}

// NewIncomingMsg creates a new incoming message
func NewIncomingMsg(channel *Channel, urn urns.URN, text string, extID string, clogUUID clogs.UUID) *MsgIn {
	now := time.Now()

	return &MsgIn{
		UUID_:        MsgUUID(uuids.NewV7()),
		ChannelUUID_: channel.UUID(),
		URN_:         urn,
		Text_:        text,
		ExternalID_:  extID,
		CreatedOn_:   now,
		ReceivedOn_:  &now,
		LogUUIDs:     []clogs.UUID{clogUUID},

		Channel_: channel,
	}
}

func (m *MsgIn) EventUUID() uuids.UUID  { return uuids.UUID(m.UUID_) }
func (m *MsgIn) UUID() MsgUUID          { return m.UUID_ }
func (m *MsgIn) ExternalID() string     { return m.ExternalID_ }
func (m *MsgIn) Text() string           { return m.Text_ }
func (m *MsgIn) Attachments() []string  { return m.Attachments_ }
func (m *MsgIn) ReceivedOn() *time.Time { return m.ReceivedOn_ }
func (m *MsgIn) URN() urns.URN          { return m.URN_ }
func (m *MsgIn) Channel() *Channel      { return m.Channel_ }

func (m *MsgIn) WithAttachment(url string) *MsgIn {
	m.Attachments_ = append(m.Attachments_, url)
	return m
}
func (m *MsgIn) WithContactName(name string) *MsgIn { m.ContactName_ = name; return m }
func (m *MsgIn) WithURNAuthTokens(tokens map[string]string) *MsgIn {
	m.URNAuthTokens_ = tokens
	return m
}
func (m *MsgIn) WithReceivedOn(date time.Time) *MsgIn { m.ReceivedOn_ = &date; return m }
func (m *MsgIn) WithNewURN(urn urns.URN, action NewURNAction) *MsgIn {
	m.NewURN_ = &NewURNSpec{Value: urn, Action: action}
	return m
}
func (m *MsgIn) WithPayload(payload json.RawMessage) *MsgIn { m.Payload_ = payload; return m }

// msgInRow is the database representation of an incoming message
type msgInRow struct {
	OrgID              OrgID          `db:"org_id"`
	UUID               MsgUUID        `db:"uuid"`
	Text               string         `db:"text"`
	Attachments        pq.StringArray `db:"attachments"`
	ExternalIdentifier null.String    `db:"external_identifier"`
	ChannelID          ChannelID      `db:"channel_id"`
	ContactID          ContactID      `db:"contact_id"`
	ContactURNID       ContactURNID   `db:"contact_urn_id"`
	CreatedOn          time.Time      `db:"created_on"`
	SentOn             *time.Time     `db:"sent_on"`
	LogUUIDs           pq.StringArray `db:"log_uuids"`
}

const sqlInsertIncomingMsg = `
INSERT INTO
	msgs_msg(org_id, uuid, direction, text, attachments, msg_type, msg_count, error_count, high_priority, status, is_android,
             visibility, external_identifier, channel_id, contact_id, contact_urn_id, created_on, modified_on, sent_on, log_uuids)
    VALUES(:org_id, :uuid, 'I', :text, :attachments, 'T', 1, 0, FALSE, 'P', FALSE,
             'V', :external_identifier, :channel_id, :contact_id, :contact_urn_id, :created_on, :created_on, :sent_on, :log_uuids)`

// InsertIncomingMsg inserts the passed in incoming message into the database
func InsertIncomingMsg(ctx context.Context, db *sqlx.DB, m *MsgIn, contact *Contact) error {
	logUUIDs := make(pq.StringArray, len(m.LogUUIDs))
	for i := range m.LogUUIDs {
		logUUIDs[i] = string(m.LogUUIDs[i])
	}

	row := &msgInRow{
		OrgID:              m.Channel_.OrgID(),
		UUID:               m.UUID_,
		Text:               m.Text_,
		Attachments:        pq.StringArray(m.Attachments_),
		ExternalIdentifier: null.String(m.ExternalID_),
		ChannelID:          m.Channel_.ID(),
		ContactID:          contact.ID_,
		ContactURNID:       contact.URNID_,
		CreatedOn:          m.CreatedOn_,
		SentOn:             m.ReceivedOn_,
		LogUUIDs:           logUUIDs,
	}

	_, err := db.NamedExecContext(ctx, sqlInsertIncomingMsg, row)
	return err
}

type MsgOrigin string

const (
	MsgOriginFlow      MsgOrigin = "flow"
	MsgOriginBroadcast MsgOrigin = "broadcast"
	MsgOriginTicket    MsgOrigin = "ticket"
	MsgOriginChat      MsgOrigin = "chat"
)

const (
	QuickReplyTypeText     = "text"
	QuickReplyTypeLocation = "location"
	QuickReplyTypeForm     = "form"
	QuickReplyTypeURL      = "url"
)

type QuickReply struct {
	Type  string `json:"type"            validate:"required"`
	Text  string `json:"text,omitempty"`
	Extra string `json:"extra,omitempty"`
}

// RequiresExtra returns whether quick replies of this type need an extra value - a form ID or a URL - to be sendable
func (qr QuickReply) RequiresExtra() bool {
	return qr.Type == QuickReplyTypeForm || qr.Type == QuickReplyTypeURL
}

func (qr QuickReply) GetText() string {
	if qr.Type == QuickReplyTypeLocation && qr.Text == "" {
		return "Send Location"
	}
	if qr.Type == QuickReplyTypeForm && qr.Text == "" {
		return "Open Form"
	}
	if qr.Type == QuickReplyTypeURL && qr.Text == "" {
		return "Open Link"
	}
	return qr.Text
}

// ContactReference is information about a contact provided on queued outgoing messages
type ContactReference struct {
	ID         ContactID   `json:"id"   validate:"required"`      // for creating session timeout fires in Postgres
	UUID       ContactUUID `json:"uuid" validate:"uuid,required"` // for creating status updates in DynamoDB
	LastSeenOn *time.Time  `json:"last_seen_on,omitempty"`
	OtherURNs  []urns.URN  `json:"other_urns,omitempty"` // contact's URNs other than the one used for this message
}

// HasOtherURN returns true if the contact has the given URN in their OtherURNs list
func (c *ContactReference) HasOtherURN(urn urns.URN) bool {
	for _, u := range c.OtherURNs {
		if u.Identity() == urn.Identity() {
			return true
		}
	}
	return false
}

// FlowReference is a reference to a flow on a queued outgoing message
type FlowReference struct {
	UUID string `json:"uuid" validate:"uuid"`
	Name string `json:"name"`
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
	UserID_               UserID            `json:"user_id"`
	Origin_               MsgOrigin         `json:"origin"         validate:"required"`
	Session_              *Session          `json:"session"`

	// set when popped from the queue rather than unmarshaled
	Channel_     *Channel          `json:"-"`
	WorkerToken_ queue.WorkerToken `json:"-"`
}

func (m *MsgOut) UUID() MsgUUID                { return m.UUID_ }
func (m *MsgOut) Channel() *Channel            { return m.Channel_ }
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
func (m *MsgOut) UserID() UserID               { return m.UserID_ }
func (m *MsgOut) Session() *Session            { return m.Session_ }
func (m *MsgOut) HighPriority() bool           { return m.HighPriority_ }

// QuickRepliesToRows takes a slice of quick replies and re-organizes it into rows and columns
func QuickRepliesToRows(replies []QuickReply, maxRows, maxRowRunes, paddingRunes int) [][]QuickReply {
	// calculate rune length if it's all one row
	totalRunes := 0
	for i := range replies {
		totalRunes += utf8.RuneCountInString(replies[i].GetText()) + paddingRunes*2
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
		strRunes := utf8.RuneCountInString(reply.GetText()) + paddingRunes*2

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
