package models

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gomodule/redigo/redis"
	filetype "github.com/h2non/filetype"
	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/svclogs"
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
	UUID_        MsgUUID        `json:"uuid"`
	ChannelUUID_ ChannelUUID    `json:"channel_uuid"`
	URN_         urns.URN       `json:"urn"`
	Text_        string         `json:"text"`
	Attachments_ []string       `json:"attachments,omitempty"`
	ExternalID_  string         `json:"external_id,omitempty"`
	CreatedOn_   time.Time      `json:"created_on"`
	ReceivedOn_  *time.Time     `json:"received_on,omitempty"`
	LogUUIDs     []svclogs.UUID `json:"log_uuids"`

	// optional extras set by handlers, used to update the contact or passed to mailroom for handling
	ContactName_   string            `json:"contact_name,omitempty"`
	URNAuthTokens_ map[string]string `json:"auth_tokens,omitempty"`
	NewURN_        *NewURNSpec       `json:"new_urn,omitempty"`
	Payload_       json.RawMessage   `json:"payload,omitempty"`

	Channel_   *Channel `json:"-"`
	Duplicate_ bool     `json:"-"`
}

// NewIncomingMsg creates a new incoming message
func NewIncomingMsg(channel *Channel, urn urns.URN, text string, extID string, clog *ChannelLog) *MsgIn {
	now := time.Now()

	return &MsgIn{
		UUID_:        MsgUUID(uuids.NewV7()),
		ChannelUUID_: channel.UUID(),
		URN_:         urns.URN(dbutil.ToValidUTF8(string(urn))), // strip out invalid UTF8 and NULL chars
		Text_:        dbutil.ToValidUTF8(text),
		ExternalID_:  dbutil.ToValidUTF8(extID),
		CreatedOn_:   now,
		ReceivedOn_:  &now,
		LogUUIDs:     []svclogs.UUID{clog.UUID},

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

// dbMsgIn is the database representation of an incoming message
type dbMsgIn struct {
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

// incoming messages are created pending (folder 'P') - mailroom moves them to the inbox or handled folder when it
// handles them, and courier never writes those folders itself
const sqlInsertIncomingMsg = `
INSERT INTO
	msgs_msg(org_id, uuid, direction, text, attachments, msg_type, msg_count, error_count, high_priority, status, is_android,
             visibility, folder, external_identifier, channel_id, contact_id, contact_urn_id, created_on, modified_on, sent_on, log_uuids)
    VALUES(:org_id, :uuid, 'I', :text, :attachments, 'T', 1, 0, FALSE, 'P', FALSE,
             'V', 'P', :external_identifier, :channel_id, :contact_id, :contact_urn_id, :created_on, :created_on, :sent_on, :log_uuids)`

// InsertIncomingMsg inserts the passed in incoming message into the database
func InsertIncomingMsg(ctx context.Context, db *sqlx.DB, m *MsgIn, contact *Contact) error {
	logUUIDs := make(pq.StringArray, len(m.LogUUIDs))
	for i := range m.LogUUIDs {
		logUUIDs[i] = string(m.LogUUIDs[i])
	}

	row := &dbMsgIn{
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

// WriteMsg writes the passed in incoming message to the database, or spools it if the database is unavailable. If
// the message is detected to be a duplicate of one already received, it's marked as such and not written.
func WriteMsg(ctx context.Context, rt *runtime.Runtime, msg *MsgIn, clog *ChannelLog) error {
	// check if this message could be a duplicate and if so steal the original's UUID
	if prevUUID := checkMsgAlreadyReceived(ctx, rt, msg); prevUUID != "" {
		msg.UUID_ = prevUUID
		msg.Duplicate_ = true
		return nil
	}

	timeout, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	if err := writeMsg(timeout, rt, msg, clog); err != nil {
		return err
	}

	recordMsgReceived(ctx, rt, msg)

	return nil
}

func writeMsg(ctx context.Context, rt *runtime.Runtime, m *MsgIn, clog *ChannelLog) error {
	channel := m.Channel()

	// check for data: attachment URLs which need to be fetched now - fetching of other URLs can be deferred until
	// message handling and performed by calling the /ci/attachment/fetch endpoint
	for i, attURL := range m.Attachments_ {
		if strings.HasPrefix(attURL, "data:") {
			attData, err := base64.StdEncoding.DecodeString(attURL[5:])
			if err != nil {
				clog.Error(ErrorAttachmentNotDecodable())
				return fmt.Errorf("unable to decode attachment data: %w", err)
			}

			var contentType, extension string
			fileType, _ := filetype.Match(attData[:300])
			if fileType != filetype.Unknown {
				contentType = fileType.MIME.Value
				extension = fileType.Extension
			} else {
				contentType = "application/octet-stream"
				extension = "bin"
			}

			newURL, err := SaveAttachment(ctx, rt, channel, contentType, attData, extension)
			if err != nil {
				return err
			}
			m.Attachments_[i] = fmt.Sprintf("%s:%s", contentType, newURL)
		}
	}

	// try to write it our db
	contact, err := writeMsgToDB(ctx, rt, m, clog)
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			slog.Warn("duplicate incoming message detected, ignoring", "msg", m.UUID())
			return nil
		}

		// if we failed, log and write to spool
		slog.Error("error writing to db", "error", err, "msg", m.UUID())

		if err := msgSpool.Add([]*MsgIn{m}); err != nil {
			return fmt.Errorf("error writing msg to spool: %w", err)
		}
		return nil
	}

	rc := rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing msg handling", "error", err, "msg", m.UUID_, "contact", contact.ID_)
	}

	return err
}

func writeMsgToDB(ctx context.Context, rt *runtime.Runtime, m *MsgIn, clog *ChannelLog) (*Contact, error) {
	contact, err := contactForMsg(ctx, rt, m, clog)

	if err != nil {
		// our db is down, write to the spool, we will write/queue this later
		return nil, fmt.Errorf("error getting contact for message: %w", err)
	}

	if err := InsertIncomingMsg(ctx, rt.DB, m, contact); err != nil {
		return nil, fmt.Errorf("error inserting message: %w", err)
	}

	return contact, nil
}

// flushMsgs is the flush function for the msg spool - it retries writing spooled msgs to the database, returning
// those that fail again so they're respooled
func flushMsgs(ctx context.Context, rt *runtime.Runtime, batch []*MsgIn) ([]*MsgIn, error) {
	var failed []*MsgIn

	for _, m := range batch {
		ctx, cancel := context.WithTimeout(ctx, time.Second*30)
		err := flushMsg(ctx, rt, m)
		cancel()

		if err != nil {
			slog.Error("error flushing spooled msg", "error", err, "msg", m.UUID())
			failed = append(failed, m)
		}
	}

	return failed, nil
}

func flushMsg(ctx context.Context, rt *runtime.Runtime, m *MsgIn) error {
	// look up our channel
	channel, err := GetChannel(ctx, AnyChannelType, m.ChannelUUID_)
	if err != nil {
		return err
	}
	m.Channel_ = channel

	// create log tho it won't be written
	clog := NewChannelLog(ChannelLogTypeMsgReceive, channel, nil, nil)

	// try to write it our db
	contact, err := writeMsgToDB(ctx, rt, m, clog)
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			slog.Warn("duplicate incoming message detected, ignoring", "msg", m.UUID())
			return nil
		}
		return err // fail? oh well, we'll try again later
	}

	rc := rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing handling for de-spooled message", "error", err, "msg", m.UUID_, "contact", contact.ID_)
	}

	return nil
}

func msgHash(m *MsgIn) string {
	hash := sha1.Sum([]byte(m.Text_ + "|" + strings.Join(m.Attachments_, "|")))
	return hex.EncodeToString(hash[:])
}

// checks to see if this message has already been received and if so returns its UUID
func checkMsgAlreadyReceived(ctx context.Context, rt *runtime.Runtime, m *MsgIn) MsgUUID {
	rc := rt.VK.Get()
	defer rc.Close()

	// if we have an external id use that
	if m.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if uuid, _ := receivedExternalIDs.Get(ctx, rc, fingerprint); uuid != "" {
			return MsgUUID(uuid)
		}
	} else {
		// otherwise de-dup based on text received from that channel+urn since last send
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if uuidAndHash, _ := receivedMsgs.Get(ctx, rc, fingerprint); uuidAndHash != "" {
			prevUUID := uuidAndHash[:36]
			prevHash := uuidAndHash[37:]

			// if it is the same hash, return the UUID
			if prevHash == msgHash(m) {
				return MsgUUID(prevUUID)
			}
		}
	}

	return ""
}

// records that the given message has been received and written to the database
func recordMsgReceived(ctx context.Context, rt *runtime.Runtime, m *MsgIn) {
	rc := rt.VK.Get()
	defer rc.Close()

	if m.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if err := receivedExternalIDs.Set(ctx, rc, fingerprint, string(m.UUID())); err != nil {
			slog.Error("error recording received external id", "msg", m.UUID(), "error", err)
		}
	} else {
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if err := receivedMsgs.Set(ctx, rc, fingerprint, fmt.Sprintf("%s|%s", m.UUID(), msgHash(m))); err != nil {
			slog.Error("error recording received msg", "msg", m.UUID(), "error", err)
		}
	}
}

// clearMsgSeen clears our seen incoming messages for the passed in channel and URN
func clearMsgSeen(ctx context.Context, vc redis.Conn, m *MsgOut) {
	fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

	if err := receivedMsgs.Del(ctx, vc, fingerprint); err != nil {
		slog.Error("error clearing received msgs", "urn", m.URN().Identity(), "error", err)
	}
}

// DeleteMsgByExternalID resolves a message external id and queues a task to mailroom to delete it
func DeleteMsgByExternalID(ctx context.Context, rt *runtime.Runtime, channel *Channel, externalID string) error {
	row := rt.DB.QueryRowContext(ctx, `SELECT uuid, contact_id FROM msgs_msg WHERE channel_id = $1 AND external_identifier = $2 AND direction = 'I'`, channel.ID(), externalID)

	var msgUUID MsgUUID
	var contactID ContactID
	if err := row.Scan(&msgUUID, &contactID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error querying deleted msg: %w", err)
	}

	if msgUUID != "" && contactID != NilContactID {
		rc := rt.VK.Get()
		defer rc.Close()

		if err := queueMsgDeleted(ctx, rc, channel, msgUUID, contactID); err != nil {
			return fmt.Errorf("error queuing message deleted task: %w", err)
		}
	}

	return nil
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

// PopNextOutgoingMsg pops the next message that needs to be sent, callers should call OnSendComplete with the
// returned message when they have dealt with the message (regardless of whether it was sent or not)
func PopNextOutgoingMsg(ctx context.Context, rt *runtime.Runtime) (*MsgOut, error) {
	vc := rt.VK.Get()
	defer vc.Close()

	markComplete := func(token queue.WorkerToken) {
		if err := queue.MarkComplete(vc, MsgQueueName, token); err != nil {
			slog.Error("error marking queue task complete", "error", err)
		}
	}

	// pop the next message off our queue
	token, msgJSON, err := queue.PopFromQueue(vc, MsgQueueName)
	if err != nil {
		return nil, err
	}

	for token == queue.Retry {
		token, msgJSON, err = queue.PopFromQueue(vc, MsgQueueName)
		if err != nil {
			return nil, err
		}
	}

	if msgJSON == "" {
		return nil, nil
	}

	msg := &MsgOut{}
	if err := json.Unmarshal([]byte(msgJSON), msg); err != nil {
		markComplete(token)
		return nil, fmt.Errorf("unable to unmarshal message: %s: %w", string(msgJSON), err)
	}

	if err := utils.Validate(msg); err != nil {
		markComplete(token)
		return nil, fmt.Errorf("queued message failed validation: %s: %w", string(msgJSON), err)
	}

	// populate the channel on our msg object
	channel, err := GetChannel(ctx, AnyChannelType, msg.ChannelUUID_)
	if err != nil {
		markComplete(token)
		return nil, err
	}

	// add some extra info to the popped message
	msg.Channel_ = channel
	msg.WorkerToken_ = token

	// clear out our seen incoming messages
	clearMsgSeen(ctx, vc, msg)

	return msg, nil
}

// WasMsgSent returns whether we think the passed in message was already sent - used as a failsafe against double
// sending messages (say if they were double queued)
func WasMsgSent(ctx context.Context, rt *runtime.Runtime, uuid MsgUUID) (bool, error) {
	rc := rt.VK.Get()
	defer rc.Close()

	return sentMsgs.IsMember(ctx, rc, string(uuid))
}

// ClearMsgSent clears any internal status that a message was previously sent - used in the case where a message is
// being forced in being resent by a user
func ClearMsgSent(ctx context.Context, rt *runtime.Runtime, uuid MsgUUID) error {
	rc := rt.VK.Get()
	defer rc.Close()

	return sentMsgs.Rem(ctx, rc, string(uuid))
}

// OnSendComplete is called when the sender has finished trying to send a message, with the new URN from the send
// result if there was one
func OnSendComplete(ctx context.Context, rt *runtime.Runtime, msg *MsgOut, status *StatusUpdate, newURN urns.URN, clog *ChannelLog) {
	log := slog.With("channel", msg.Channel().UUID(), "msg", msg.UUID(), "clog", clog.UUID, "status", status)

	rc := rt.VK.Get()
	defer rc.Close()

	if err := queue.MarkComplete(rc, MsgQueueName, msg.WorkerToken_); err != nil {
		log.Error("unable to mark queue task complete", "error", err)
	}

	// if message won't be retried, mark as sent to avoid dupe sends
	if status.Status() != MsgStatusErrored {
		if err := sentMsgs.Add(ctx, rc, string(msg.UUID())); err != nil {
			log.Error("unable to mark message sent", "error", err)
		}
	}

	// if message was successfully sent, and we have a session timeout, update it
	wasSuccess := status.Status() == MsgStatusWired || status.Status() == MsgStatusSent || status.Status() == MsgStatusDelivered || status.Status() == MsgStatusRead
	if wasSuccess && msg.Session_ != nil && msg.Session_.Timeout > 0 {
		if err := insertTimeoutFire(ctx, rt, msg); err != nil {
			log.Error("unable to update session timeout", "error", err, "session_uuid", msg.Session_.UUID)
		}
	}

	// if send result includes a new URN to add to the contact, queue a contact_changed task
	if wasSuccess && newURN != urns.NilURN && !msg.Contact().HasOtherURN(newURN) {
		ch := msg.Channel()
		err := queueMailroomTask(ctx, rc, "contact_changed", ch.OrgID_, msg.Contact().ID, map[string]any{
			"channel_id": ch.ID_,
			"new_urn": map[string]string{
				"value":  newURN.String(),
				"action": "append",
			},
		})
		if err != nil {
			log.Error("unable to queue contact_changed task", "error", err)
		}
	}

	rt.Stats.RecordOutgoing(string(msg.Channel().ChannelType()), wasSuccess, clog.Elapsed)
}

const sqlInsertTimeoutFire = `
INSERT INTO contacts_contactfire(org_id, contact_id, fire_type, scope, fire_on, session_uuid, sprint_uuid)
                          VALUES($1, $2, 'T', '', $3, $4, $5)
ON CONFLICT DO NOTHING`

// insertTimeoutFire inserts a timeout fire for the session associated with the given msg
func insertTimeoutFire(ctx context.Context, rt *runtime.Runtime, m *MsgOut) error {
	timeoutOn := dates.Now().Add(time.Duration(m.Session_.Timeout) * time.Second)

	_, err := rt.DB.ExecContext(ctx, sqlInsertTimeoutFire, m.OrgID_, m.Contact_.ID, timeoutOn, m.Session_.UUID, m.Session_.SprintUUID)
	if err != nil {
		return fmt.Errorf("error inserting session timeout contact fire for session %s: %w", m.Session_.UUID, err)
	}
	return nil
}

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
