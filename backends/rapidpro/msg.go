package rapidpro

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	filetype "github.com/h2non/filetype"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/utils/queue"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// Msg is our base struct to represent msgs both in our JSON and db representations
type Msg struct {
	OrgID_        models.OrgID         `json:"org_id"          db:"org_id"`
	ID_           models.MsgID         `json:"id"              db:"id"`
	UUID_         models.MsgUUID       `json:"uuid"            db:"uuid"`
	Direction_    models.MsgDirection  `                       db:"direction"`
	Status_       models.MsgStatus     `                       db:"status"`
	Visibility_   models.MsgVisibility `                       db:"visibility"`
	HighPriority_ bool                 `json:"high_priority"   db:"high_priority"`
	Text_         string               `json:"text"            db:"text"`
	Attachments_  pq.StringArray       `json:"attachments"     db:"attachments"`
	QuickReplies_ []models.QuickReply  `json:"quick_replies"`
	Locale_       null.String          `json:"locale"          db:"locale"`
	Templating_   *models.Templating   `json:"templating"      db:"templating"`
	ExternalID_   null.String          `                       db:"external_id"`
	ChannelID_    models.ChannelID     `                       db:"channel_id"`
	ContactID_    models.ContactID     `json:"contact_id"      db:"contact_id"`
	ContactURNID_ models.ContactURNID  `json:"contact_urn_id"  db:"contact_urn_id"`

	MessageCount_ int         `                     db:"msg_count"`
	ErrorCount_   int         `                     db:"error_count"`
	FailedReason_ null.String `                     db:"failed_reason"`

	NextAttempt_ time.Time      `                     db:"next_attempt"`
	CreatedOn_   time.Time      `json:"created_on"    db:"created_on"`
	ModifiedOn_  time.Time      `                     db:"modified_on"`
	SentOn_      *time.Time     `                     db:"sent_on"`
	LogUUIDs     pq.StringArray `                     db:"log_uuids"`

	// extra non-model fields that mailroom will include in queued payload
	ChannelUUID_          models.ChannelUUID     `json:"channel_uuid"`
	URN_                  urns.URN               `json:"urn"`
	URNAuth_              string                 `json:"urn_auth"`
	ResponseToExternalID_ string                 `json:"response_to_external_id"`
	IsResend_             bool                   `json:"is_resend"`
	Flow_                 *models.FlowReference  `json:"flow"`
	OptIn_                *models.OptInReference `json:"optin"`
	UserID_               models.UserID          `json:"user_id"`
	Origin_               models.MsgOrigin       `json:"origin"`
	ContactLastSeenOn_    *time.Time             `json:"contact_last_seen_on"`
	Session_              *models.Session        `json:"session"`

	ContactName_   string            `json:"contact_name"`
	URNAuthTokens_ map[string]string `json:"auth_tokens"`
	channel        *models.Channel
	workerToken    queue.WorkerToken
	alreadyWritten bool
}

// newMsg creates a new DBMsg object with the passed in parameters
func newMsg(direction models.MsgDirection, channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) *Msg {
	now := time.Now()
	dbChannel := channel.(*models.Channel)

	return &Msg{
		OrgID_:        dbChannel.OrgID(),
		UUID_:         models.MsgUUID(uuids.NewV7()),
		Direction_:    direction,
		Status_:       models.MsgStatusPending,
		Visibility_:   models.MsgVisible,
		HighPriority_: false,
		Text_:         text,
		ExternalID_:   null.String(extID),

		ChannelID_:   dbChannel.ID(),
		ChannelUUID_: dbChannel.UUID(),

		URN_:          urn,
		MessageCount_: 1,

		CreatedOn_:  now,
		ModifiedOn_: now,
		LogUUIDs:    []string{string(clog.UUID)},

		channel:        dbChannel,
		workerToken:    "",
		alreadyWritten: false,
	}
}

func (m *Msg) EventID() int64           { return int64(m.ID_) }
func (m *Msg) ID() models.MsgID         { return m.ID_ }
func (m *Msg) UUID() models.MsgUUID     { return m.UUID_ }
func (m *Msg) ExternalID() string       { return string(m.ExternalID_) }
func (m *Msg) Text() string             { return m.Text_ }
func (m *Msg) Attachments() []string    { return m.Attachments_ }
func (m *Msg) URN() urns.URN            { return m.URN_ }
func (m *Msg) Channel() courier.Channel { return m.channel }

// outgoing specific
func (m *Msg) QuickReplies() []models.QuickReply { return m.QuickReplies_ }
func (m *Msg) Locale() i18n.Locale               { return i18n.Locale(string(m.Locale_)) }
func (m *Msg) Templating() *models.Templating    { return m.Templating_ }
func (m *Msg) URNAuth() string                   { return m.URNAuth_ }
func (m *Msg) Origin() models.MsgOrigin          { return m.Origin_ }
func (m *Msg) ContactLastSeenOn() *time.Time     { return m.ContactLastSeenOn_ }
func (m *Msg) ResponseToExternalID() string      { return m.ResponseToExternalID_ }
func (m *Msg) SentOn() *time.Time                { return m.SentOn_ }
func (m *Msg) IsResend() bool                    { return m.IsResend_ }
func (m *Msg) Flow() *models.FlowReference       { return m.Flow_ }
func (m *Msg) OptIn() *models.OptInReference     { return m.OptIn_ }
func (m *Msg) UserID() models.UserID             { return m.UserID_ }
func (m *Msg) Session() *models.Session          { return m.Session_ }
func (m *Msg) HighPriority() bool                { return m.HighPriority_ }

// incoming specific
func (m *Msg) ReceivedOn() *time.Time { return m.SentOn_ }
func (m *Msg) WithAttachment(url string) courier.MsgIn {
	m.Attachments_ = append(m.Attachments_, url)
	return m
}
func (m *Msg) WithContactName(name string) courier.MsgIn { m.ContactName_ = name; return m }
func (m *Msg) WithURNAuthTokens(tokens map[string]string) courier.MsgIn {
	m.URNAuthTokens_ = tokens
	return m
}
func (m *Msg) WithReceivedOn(date time.Time) courier.MsgIn { m.SentOn_ = &date; return m }

func (m *Msg) hash() string {
	hash := sha1.Sum([]byte(m.Text_ + "|" + strings.Join(m.Attachments_, "|")))
	return hex.EncodeToString(hash[:])
}

// WriteMsg creates a message given the passed in arguments
func writeMsg(ctx context.Context, b *backend, m *Msg, clog *courier.ChannelLog) error {
	// this msg has already been written (we received it twice), we are a no op
	if m.alreadyWritten {
		return nil
	}

	channel := m.Channel()

	// check for data: attachment URLs which need to be fetched now - fetching of other URLs can be deferred until
	// message handling and performed by calling the /c/_fetch-attachment endpoint
	for i, attURL := range m.Attachments_ {
		if strings.HasPrefix(attURL, "data:") {
			attData, err := base64.StdEncoding.DecodeString(attURL[5:])
			if err != nil {
				clog.Error(courier.ErrorAttachmentNotDecodable())
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

			newURL, err := b.SaveAttachment(ctx, channel, contentType, attData, extension)
			if err != nil {
				return err
			}
			m.Attachments_[i] = fmt.Sprintf("%s:%s", contentType, newURL)
		}
	}

	// try to write it our db
	contact, err := writeMsgToDB(ctx, b, m, clog)
	if err != nil {
		// if we failed, log and write to spool
		slog.Error("error writing to db", "error", err, "msg", m.UUID())

		if err := courier.WriteToSpool(b.rt.Config.SpoolDir, "msgs", m); err != nil {
			return fmt.Errorf("error writing msg to spool: %w", err)
		}
		return nil
	}

	rc := b.rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing msg handling", "error", err, "msg", m.ID_, "contact", contact.ID_)
	}

	return err
}

const sqlInsertMsg = `
INSERT INTO
	msgs_msg(org_id, uuid, direction, text, attachments, msg_type, msg_count, error_count, high_priority, status, is_android,
             visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, sent_on, log_uuids)
    VALUES(:org_id, :uuid, :direction, :text, :attachments, 'T', :msg_count, :error_count, :high_priority, :status, FALSE,
           :visibility, :external_id, :channel_id, :contact_id, :contact_urn_id, :created_on, :modified_on, :next_attempt, :sent_on, :log_uuids)
RETURNING id`

func writeMsgToDB(ctx context.Context, b *backend, m *Msg, clog *courier.ChannelLog) (*models.Contact, error) {
	contact, err := contactForURN(ctx, b, m.OrgID_, m.channel, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)

	if err != nil {
		// our db is down, write to the spool, we will write/queue this later
		return nil, fmt.Errorf("error getting contact for message: %w", err)
	}

	// set our contact and urn id
	m.ContactID_ = contact.ID_
	m.ContactURNID_ = contact.URNID_

	rows, err := b.rt.DB.NamedQueryContext(ctx, sqlInsertMsg, m)
	if err != nil {
		return nil, fmt.Errorf("error inserting message: %w", err)
	}
	defer rows.Close()

	rows.Next()

	if err := rows.Scan(&m.ID_); err != nil {
		return nil, fmt.Errorf("error scanning for inserted message id: %w", err)
	}

	return contact, nil
}

func checkDuplicate(ctx context.Context, rt *runtime.Runtime, uuid models.MsgUUID) (bool, error) {
	row := rt.DB.QueryRowContext(ctx, `SELECT id FROM msgs_msg WHERE uuid = $1 LIMIT 1`, uuid)

	var id models.MsgID
	err := row.Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("error checking for duplicate message: %w", err)
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return true, nil
}

//-----------------------------------------------------------------------------
// Msg flusher for flushing failed writes
//-----------------------------------------------------------------------------

func (b *backend) flushMsgFile(filename string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	m := &Msg{}
	err := json.Unmarshal(contents, m)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// look up our channel
	channel, err := b.GetChannel(ctx, models.AnyChannelType, m.ChannelUUID_)
	if err != nil {
		return err
	}
	m.channel = channel.(*models.Channel)

	// create log tho it won't be written
	clog := courier.NewChannelLog(courier.ChannelLogTypeMsgReceive, channel, nil)

	// try to write it our db
	contact, err := writeMsgToDB(ctx, b, m, clog)
	if err != nil {
		duplicate, dupErr := checkDuplicate(ctx, b.rt, m.UUID_)
		if dupErr != nil {
			slog.Error("error checking for duplicate message", "error", dupErr, "msg", m.UUID())
			return dupErr // fail? oh well, we'll try again later
		}
		if !duplicate {
			return err // fail? oh well, we'll try again later
		}
	}

	rc := b.rt.VK.Get()
	defer rc.Close()

	// queue to mailroom for handling
	if err := queueMsgHandling(ctx, rc, contact, m); err != nil {
		slog.Error("error queueing handling for de-spooled message", "error", err, "msg", m.ID_, "contact", contact.ID_)
	}

	return nil
}

//-----------------------------------------------------------------------------
// Deduping utility methods
//-----------------------------------------------------------------------------

// checks to see if this message has already been received and if so returns its UUID
func (b *backend) checkMsgAlreadyReceived(ctx context.Context, m *Msg) models.MsgUUID {
	rc := b.rt.VK.Get()
	defer rc.Close()

	// if we have an external id use that
	if m.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if uuid, _ := b.receivedExternalIDs.Get(ctx, rc, fingerprint); uuid != "" {
			return models.MsgUUID(uuid)
		}
	} else {
		// otherwise de-dup based on text received from that channel+urn since last send
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if uuidAndHash, _ := b.receivedMsgs.Get(ctx, rc, fingerprint); uuidAndHash != "" {
			prevUUID := uuidAndHash[:36]
			prevHash := uuidAndHash[37:]

			// if it is the same hash, return the UUID
			if prevHash == m.hash() {
				return models.MsgUUID(prevUUID)
			}
		}
	}

	return models.NilMsgUUID
}

// records that the given message has been received and written to the database
func (b *backend) recordMsgReceived(ctx context.Context, m *Msg) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	if m.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", m.Channel().UUID(), m.URN().Identity(), m.ExternalID())

		if err := b.receivedExternalIDs.Set(ctx, rc, fingerprint, string(m.UUID())); err != nil {
			slog.Error("error recording received external id", "msg", m.UUID(), "error", err)
		}
	} else {
		fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

		if err := b.receivedMsgs.Set(ctx, rc, fingerprint, fmt.Sprintf("%s|%s", m.UUID(), m.hash())); err != nil {
			slog.Error("error recording received msg", "msg", m.UUID(), "error", err)
		}
	}
}

// clearMsgSeen clears our seen incoming messages for the passed in channel and URN
func (b *backend) clearMsgSeen(ctx context.Context, m *Msg) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

	if err := b.receivedMsgs.Del(ctx, rc, fingerprint); err != nil {
		slog.Error("error clearing received msgs", "urn", m.URN().Identity(), "error", err)
	}
}
