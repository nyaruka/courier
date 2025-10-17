package rapidpro

import (
	"context"
	"crypto/sha1"
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
	"github.com/nyaruka/courier/utils/queue"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// MsgIn is an incoming message which can be written to the database or marshaled to a spool file
type MsgIn struct {
	OrgID_        models.OrgID        `db:"org_id"         json:"org_id"`
	ID_           models.MsgID        `db:"id"             json:"id"`
	UUID_         models.MsgUUID      `db:"uuid"           json:"uuid"`
	Text_         string              `db:"text"           json:"text"`
	Attachments_  pq.StringArray      `db:"attachments"    json:"attachments"`
	ExternalID_   null.String         `db:"external_id"    json:"external_id"`
	ChannelID_    models.ChannelID    `db:"channel_id"     json:"channel_id"`
	ContactID_    models.ContactID    `db:"contact_id"     json:"contact_id"`
	ContactURNID_ models.ContactURNID `db:"contact_urn_id" json:"contact_urn_id"`
	CreatedOn_    time.Time           `db:"created_on"     json:"created_on"`
	ModifiedOn_   time.Time           `db:"modified_on"    json:"modified_on"`
	SentOn_       *time.Time          `db:"sent_on"        json:"sent_on"`
	LogUUIDs      pq.StringArray      `db:"log_uuids"      json:"log_uuids"`

	// extra non-model fields needed for queueing to mailroom
	ChannelUUID_   models.ChannelUUID `json:"channel_uuid"`
	URN_           urns.URN           `json:"urn"`
	ContactName_   string             `json:"contact_name"`
	URNAuthTokens_ map[string]string  `json:"auth_tokens"`

	channel        *models.Channel
	alreadyWritten bool
}

// creates a new incoming message
func newIncomingMsg(channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) *MsgIn {
	now := time.Now()
	dbChannel := channel.(*models.Channel)

	return &MsgIn{
		OrgID_:      dbChannel.OrgID(),
		UUID_:       models.MsgUUID(uuids.NewV7()),
		Text_:       text,
		ExternalID_: null.String(extID),
		ChannelID_:  dbChannel.ID(),
		CreatedOn_:  now,
		ModifiedOn_: now,
		LogUUIDs:    pq.StringArray{string(clog.UUID)},

		URN_:           urn,
		channel:        dbChannel,
		alreadyWritten: false,
	}
}

func (m *MsgIn) EventUUID() uuids.UUID    { return uuids.UUID(m.UUID_) }
func (m *MsgIn) ID() models.MsgID         { return m.ID_ }
func (m *MsgIn) UUID() models.MsgUUID     { return m.UUID_ }
func (m *MsgIn) ExternalID() string       { return string(m.ExternalID_) }
func (m *MsgIn) Text() string             { return m.Text_ }
func (m *MsgIn) Attachments() []string    { return []string(m.Attachments_) }
func (m *MsgIn) URN() urns.URN            { return m.URN_ }
func (m *MsgIn) Channel() courier.Channel { return m.channel }

func (m *MsgIn) ReceivedOn() *time.Time { return m.SentOn_ }
func (m *MsgIn) WithAttachment(url string) courier.MsgIn {
	m.Attachments_ = append(m.Attachments_, url)
	return m
}
func (m *MsgIn) WithContactName(name string) courier.MsgIn { m.ContactName_ = name; return m }
func (m *MsgIn) WithURNAuthTokens(tokens map[string]string) courier.MsgIn {
	m.URNAuthTokens_ = tokens
	return m
}
func (m *MsgIn) WithReceivedOn(date time.Time) courier.MsgIn { m.SentOn_ = &date; return m }

func (m *MsgIn) hash() string {
	hash := sha1.Sum([]byte(m.Text_ + "|" + strings.Join(m.Attachments_, "|")))
	return hex.EncodeToString(hash[:])
}

// WriteMsg creates a message given the passed in arguments
func writeMsg(ctx context.Context, b *backend, m *MsgIn, clog *courier.ChannelLog) error {
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
             visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, sent_on, log_uuids)
    VALUES(:org_id, :uuid, 'I', :text, :attachments, 'T', 1, 0, FALSE, 'P', FALSE,
             'V', :external_id, :channel_id, :contact_id, :contact_urn_id, :created_on, :modified_on, :sent_on, :log_uuids)
RETURNING id`

func writeMsgToDB(ctx context.Context, b *backend, m *MsgIn, clog *courier.ChannelLog) (*models.Contact, error) {
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

//-----------------------------------------------------------------------------
// Msg flusher for flushing failed writes
//-----------------------------------------------------------------------------

func (b *backend) flushMsgFile(filename string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	m := &MsgIn{}
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
		return err // fail? oh well, we'll try again later
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
func (b *backend) checkMsgAlreadyReceived(ctx context.Context, m *MsgIn) models.MsgUUID {
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

	return ""
}

// records that the given message has been received and written to the database
func (b *backend) recordMsgReceived(ctx context.Context, m *MsgIn) {
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
func (b *backend) clearMsgSeen(ctx context.Context, m *MsgOut) {
	rc := b.rt.VK.Get()
	defer rc.Close()

	fingerprint := fmt.Sprintf("%s|%s", m.Channel().UUID(), m.URN().Identity())

	if err := b.receivedMsgs.Del(ctx, rc, fingerprint); err != nil {
		slog.Error("error clearing received msgs", "urn", m.URN().Identity(), "error", err)
	}
}

type MsgOut struct {
	OrgID_                models.OrgID             `json:"org_id"         validate:"required"`
	ID_                   models.MsgID             `json:"id"             validate:"required"`
	UUID_                 models.MsgUUID           `json:"uuid"           validate:"required"`
	Contact_              *models.ContactReference `json:"contact"        validate:"required"`
	HighPriority_         bool                     `json:"high_priority"`
	Text_                 string                   `json:"text"`
	Attachments_          []string                 `json:"attachments"`
	QuickReplies_         []models.QuickReply      `json:"quick_replies"`
	Locale_               i18n.Locale              `json:"locale"`
	Templating_           *models.Templating       `json:"templating"`
	CreatedOn_            time.Time                `json:"created_on"     validate:"required"`
	ChannelUUID_          models.ChannelUUID       `json:"channel_uuid"   validate:"required"`
	URN_                  urns.URN                 `json:"urn"            validate:"required"`
	URNAuth_              string                   `json:"urn_auth"`
	ResponseToExternalID_ string                   `json:"response_to_external_id"`
	IsResend_             bool                     `json:"is_resend"`
	Flow_                 *models.FlowReference    `json:"flow"`
	OptIn_                *models.OptInReference   `json:"optin"`
	UserID_               models.UserID            `json:"user_id"`
	Origin_               models.MsgOrigin         `json:"origin"         validate:"required"`
	Session_              *models.Session          `json:"session"`

	channel     *models.Channel
	workerToken queue.WorkerToken
}

func (m *MsgOut) EventUUID() uuids.UUID             { return uuids.UUID(m.UUID_) }
func (m *MsgOut) ID() models.MsgID                  { return m.ID_ }
func (m *MsgOut) UUID() models.MsgUUID              { return m.UUID_ }
func (m *MsgOut) Contact() *models.ContactReference { return m.Contact_ }
func (m *MsgOut) Text() string                      { return m.Text_ }
func (m *MsgOut) Attachments() []string             { return m.Attachments_ }
func (m *MsgOut) URN() urns.URN                     { return m.URN_ }
func (m *MsgOut) Channel() courier.Channel          { return m.channel }
func (m *MsgOut) QuickReplies() []models.QuickReply { return m.QuickReplies_ }
func (m *MsgOut) Locale() i18n.Locale               { return m.Locale_ }
func (m *MsgOut) Templating() *models.Templating    { return m.Templating_ }
func (m *MsgOut) URNAuth() string                   { return m.URNAuth_ }
func (m *MsgOut) Origin() models.MsgOrigin          { return m.Origin_ }
func (m *MsgOut) ResponseToExternalID() string      { return m.ResponseToExternalID_ }
func (m *MsgOut) IsResend() bool                    { return m.IsResend_ }
func (m *MsgOut) Flow() *models.FlowReference       { return m.Flow_ }
func (m *MsgOut) OptIn() *models.OptInReference     { return m.OptIn_ }
func (m *MsgOut) UserID() models.UserID             { return m.UserID_ }
func (m *MsgOut) Session() *models.Session          { return m.Session_ }
func (m *MsgOut) HighPriority() bool                { return m.HighPriority_ }
