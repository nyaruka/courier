package rapidpro

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	filetype "gopkg.in/h2non/filetype.v1"
)

// MsgDirection is the direction of a message
type MsgDirection string

// Possible values for MsgDirection
const (
	MsgIncoming     MsgDirection = "I"
	MsgOutgoing     MsgDirection = "O"
	NilMsgDirection MsgDirection = ""
)

// MsgVisibility is the visibility of a message
type MsgVisibility string

// Possible values for MsgVisibility
const (
	MsgVisible  MsgVisibility = "V"
	MsgDeleted  MsgVisibility = "D"
	MsgArchived MsgVisibility = "A"
)

// WriteMsg creates a message given the passed in arguments
func writeMsg(ctx context.Context, b *backend, msg courier.Msg, clog *courier.ChannelLog) error {
	m := msg.(*DBMsg)

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
				return errors.Wrap(err, "unable to decode attachment data")
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
	err := writeMsgToDB(ctx, b, m, clog)

	// fail? log
	if err != nil {
		logrus.WithError(err).WithField("msg", m.UUID()).Error("error writing to db")
	}

	// if we failed write to spool
	if err != nil {
		err = courier.WriteToSpool(b.config.SpoolDir, "msgs", m)
	}

	// mark this msg as having been seen
	b.writeMsgSeen(m)

	return err
}

// newMsg creates a new DBMsg object with the passed in parameters
func newMsg(direction MsgDirection, channel courier.Channel, urn urns.URN, text string, extID string, clog *courier.ChannelLog) *DBMsg {
	now := time.Now()
	dbChannel := channel.(*DBChannel)

	return &DBMsg{
		OrgID_:        dbChannel.OrgID(),
		UUID_:         courier.MsgUUID(uuids.New()),
		Direction_:    direction,
		Status_:       courier.MsgPending,
		Visibility_:   MsgVisible,
		HighPriority_: false,
		Text_:         text,
		ExternalID_:   null.String(extID),

		ChannelID_:   dbChannel.ID(),
		ChannelUUID_: dbChannel.UUID(),

		URN_:          urn,
		MessageCount_: 1,

		NextAttempt_: now,
		CreatedOn_:   now,
		ModifiedOn_:  now,
		QueuedOn_:    now,
		LogUUIDs:     []string{string(clog.UUID())},

		channel:        dbChannel,
		workerToken:    "",
		alreadyWritten: false,
	}
}

const sqlInsertMsg = `
INSERT INTO
	msgs_msg(org_id, uuid, direction, text, attachments, msg_type, msg_count, error_count, high_priority, status,
             visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, queued_on, sent_on, log_uuids)
    VALUES(:org_id, :uuid, :direction, :text, :attachments, 'T', :msg_count, :error_count, :high_priority, :status,
           :visibility, :external_id, :channel_id, :contact_id, :contact_urn_id, :created_on, :modified_on, :next_attempt, :queued_on, :sent_on, :log_uuids)
RETURNING id`

func writeMsgToDB(ctx context.Context, b *backend, m *DBMsg, clog *courier.ChannelLog) error {
	// grab the contact for this msg
	contact, err := contactForURN(ctx, b, m.OrgID_, m.channel, m.URN_, m.URNAuth_, m.contactName, clog)

	// our db is down, write to the spool, we will write/queue this later
	if err != nil {
		return errors.Wrap(err, "error getting contact for message")
	}

	// set our contact and urn id
	m.ContactID_ = contact.ID_
	m.ContactURNID_ = contact.URNID_

	rows, err := b.db.NamedQueryContext(ctx, sqlInsertMsg, m)
	if err != nil {
		return errors.Wrap(err, "error inserting message")
	}
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&m.ID_)
	if err != nil {
		return errors.Wrap(err, "error scanning for inserted message id")
	}

	// queue this up to be handled by RapidPro
	rc := b.redisPool.Get()
	defer rc.Close()
	err = queueMsgHandling(rc, contact, m)

	// if we had a problem queueing the handling, log it, but our message is written, it'll
	// get picked up by our rapidpro catch-all after a period
	if err != nil {
		logrus.WithError(err).WithField("msg_id", m.ID_).Error("error queueing msg handling")
	}

	return nil
}

const sqlSelectMsg = `
SELECT
	org_id,
	direction,
	text,
	attachments,
	quick_replies,
	msg_count,
	error_count,
	failed_reason,
	high_priority,
	status,
	visibility,
	external_id,
	channel_id,
	contact_id,
	contact_urn_id,
	created_on,
	modified_on,
	next_attempt,
	queued_on,
	sent_on,
	log_uuids
FROM
	msgs_msg
WHERE
	id = $1`

const selectChannelSQL = `
SELECT
	org_id,
	ch.id as id,
	ch.uuid as uuid,
	ch.name as name,
	channel_type, schemes,
	address, role,
	ch.country as country,
	ch.config as config,
	org.config as org_config,
	org.is_anon as org_is_anon
FROM
	channels_channel ch
	JOIN orgs_org org on ch.org_id = org.id
WHERE
    ch.id = $1
`

//-----------------------------------------------------------------------------
// Msg flusher for flushing failed writes
//-----------------------------------------------------------------------------

func (b *backend) flushMsgFile(filename string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	msg := &DBMsg{}
	err := json.Unmarshal(contents, msg)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// look up our channel
	channel, err := b.GetChannel(ctx, courier.AnyChannelType, msg.ChannelUUID_)
	if err != nil {
		return err
	}
	msg.channel = channel.(*DBChannel)

	// create log tho it won't be written
	clog := courier.NewChannelLog(courier.ChannelLogTypeMsgReceive, channel, nil)

	// try to write it our db
	err = writeMsgToDB(ctx, b, msg, clog)

	// fail? oh well, we'll try again later
	return err
}

//-----------------------------------------------------------------------------
// Deduping utility methods
//-----------------------------------------------------------------------------

// checks to see if this message has already been seen and if so returns its UUID
func (b *backend) checkMsgSeen(msg *DBMsg) courier.MsgUUID {
	rc := b.redisPool.Get()
	defer rc.Close()

	// if we have an external id use that
	if msg.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", msg.Channel().UUID(), msg.URN().Identity(), msg.ExternalID())

		uuid, _ := b.seenExternalIDs.Get(rc, fingerprint)

		if uuid != "" {
			return courier.MsgUUID(uuid)
		}
	} else {
		// otherwise de-dup based on text received from that channel+urn since last send
		fingerprint := fmt.Sprintf("%s|%s", msg.Channel().UUID(), msg.URN().Identity())

		uuidAndText, _ := b.seenMsgs.Get(rc, fingerprint)

		// if we have seen a message from this channel+urn check text too
		if uuidAndText != "" {
			prevText := uuidAndText[37:]

			// if it is the same, return the UUID
			if prevText == msg.Text() {
				return courier.MsgUUID(uuidAndText[:36])
			}
		}
	}

	return courier.NilMsgUUID
}

// writeMsgSeen records that the given message has been seen and written to the database
func (b *backend) writeMsgSeen(msg *DBMsg) {
	rc := b.redisPool.Get()
	defer rc.Close()

	if msg.ExternalID_ != "" {
		fingerprint := fmt.Sprintf("%s|%s|%s", msg.Channel().UUID(), msg.URN().Identity(), msg.ExternalID())

		b.seenExternalIDs.Set(rc, fingerprint, string(msg.UUID()))
	} else {
		fingerprint := fmt.Sprintf("%s|%s", msg.Channel().UUID(), msg.URN().Identity())

		b.seenMsgs.Set(rc, fingerprint, fmt.Sprintf("%s|%s", msg.UUID(), msg.Text()))
	}
}

// clearMsgSeen clears our seen incoming messages for the passed in channel and URN
func (b *backend) clearMsgSeen(rc redis.Conn, msg *DBMsg) {
	fingerprint := fmt.Sprintf("%s|%s", msg.Channel().UUID(), msg.URN().Identity())

	b.seenMsgs.Remove(rc, fingerprint)
}

//-----------------------------------------------------------------------------
// Our implementation of Msg interface
//-----------------------------------------------------------------------------

// DBMsg is our base struct to represent msgs both in our JSON and db representations
type DBMsg struct {
	OrgID_        OrgID                  `json:"org_id"          db:"org_id"`
	ID_           courier.MsgID          `json:"id"              db:"id"`
	UUID_         courier.MsgUUID        `json:"uuid"            db:"uuid"`
	Direction_    MsgDirection           `                       db:"direction"`
	Status_       courier.MsgStatusValue `                       db:"status"`
	Visibility_   MsgVisibility          `                       db:"visibility"`
	HighPriority_ bool                   `json:"high_priority"   db:"high_priority"`
	Text_         string                 `json:"text"            db:"text"`
	Attachments_  pq.StringArray         `json:"attachments"     db:"attachments"`
	QuickReplies_ pq.StringArray         `json:"quick_replies"   db:"quick_replies"`
	Locale_       null.String            `json:"locale"          db:"locale"`
	ExternalID_   null.String            `                       db:"external_id"`
	Metadata_     json.RawMessage        `json:"metadata"        db:"metadata"`

	ChannelID_    courier.ChannelID `                       db:"channel_id"`
	ContactID_    ContactID         `json:"contact_id"      db:"contact_id"`
	ContactURNID_ ContactURNID      `json:"contact_urn_id"  db:"contact_urn_id"`

	MessageCount_ int         `                     db:"msg_count"`
	ErrorCount_   int         `                     db:"error_count"`
	FailedReason_ null.String `                     db:"failed_reason"`

	NextAttempt_ time.Time      `                     db:"next_attempt"`
	CreatedOn_   time.Time      `json:"created_on"    db:"created_on"`
	ModifiedOn_  time.Time      `                     db:"modified_on"`
	QueuedOn_    time.Time      `                     db:"queued_on"`
	SentOn_      *time.Time     `                     db:"sent_on"`
	LogUUIDs     pq.StringArray `                     db:"log_uuids"`

	// extra non-model fields that mailroom will include in queued payload
	ChannelUUID_          courier.ChannelUUID    `json:"channel_uuid"`
	URN_                  urns.URN               `json:"urn"`
	URNAuth_              string                 `json:"urn_auth"`
	ResponseToExternalID_ string                 `json:"response_to_external_id"`
	IsResend_             bool                   `json:"is_resend"`
	Flow_                 *courier.FlowReference `json:"flow"`
	Origin_               courier.MsgOrigin      `json:"origin"`
	ContactLastSeenOn_    *time.Time             `json:"contact_last_seen_on"`

	// extra fields used to allow courier to update a session's timeout to *after* the message has been sent
	SessionID_            SessionID  `json:"session_id"`
	SessionTimeout_       int        `json:"session_timeout"`
	SessionWaitStartedOn_ *time.Time `json:"session_wait_started_on"`
	SessionStatus_        string     `json:"session_status"`

	contactName    string
	channel        *DBChannel
	workerToken    queue.WorkerToken
	alreadyWritten bool
}

func (m *DBMsg) ID() courier.MsgID             { return m.ID_ }
func (m *DBMsg) EventID() int64                { return int64(m.ID_) }
func (m *DBMsg) UUID() courier.MsgUUID         { return m.UUID_ }
func (m *DBMsg) Text() string                  { return m.Text_ }
func (m *DBMsg) Attachments() []string         { return m.Attachments_ }
func (m *DBMsg) QuickReplies() []string        { return m.QuickReplies_ }
func (m *DBMsg) Locale() courier.Locale        { return courier.Locale(string(m.Locale_)) }
func (m *DBMsg) ExternalID() string            { return string(m.ExternalID_) }
func (m *DBMsg) URN() urns.URN                 { return m.URN_ }
func (m *DBMsg) URNAuth() string               { return m.URNAuth_ }
func (m *DBMsg) ContactName() string           { return m.contactName }
func (m *DBMsg) HighPriority() bool            { return m.HighPriority_ }
func (m *DBMsg) ReceivedOn() *time.Time        { return m.SentOn_ }
func (m *DBMsg) SentOn() *time.Time            { return m.SentOn_ }
func (m *DBMsg) ResponseToExternalID() string  { return m.ResponseToExternalID_ }
func (m *DBMsg) IsResend() bool                { return m.IsResend_ }
func (m *DBMsg) Channel() courier.Channel      { return m.channel }
func (m *DBMsg) SessionStatus() string         { return m.SessionStatus_ }
func (m *DBMsg) Flow() *courier.FlowReference  { return m.Flow_ }
func (m *DBMsg) Origin() courier.MsgOrigin     { return m.Origin_ }
func (m *DBMsg) ContactLastSeenOn() *time.Time { return m.ContactLastSeenOn_ }

func (m *DBMsg) FlowName() string {
	if m.Flow_ == nil {
		return ""
	}
	return m.Flow_.Name
}

func (m *DBMsg) FlowUUID() string {
	if m.Flow_ == nil {
		return ""
	}
	return m.Flow_.UUID
}

func (m *DBMsg) Topic() string {
	if m.Metadata_ == nil {
		return ""
	}
	topic, _, _, _ := jsonparser.Get(m.Metadata_, "topic")
	return string(topic)
}

// Metadata returns the metadata for this message
func (m *DBMsg) Metadata() json.RawMessage {
	return m.Metadata_
}

// WithContactName can be used to set the contact name on a msg
func (m *DBMsg) WithContactName(name string) courier.Msg { m.contactName = name; return m }

// WithReceivedOn can be used to set sent_on on a msg in a chained call
func (m *DBMsg) WithReceivedOn(date time.Time) courier.Msg { m.SentOn_ = &date; return m }

// WithID can be used to set the id on a msg in a chained call
func (m *DBMsg) WithID(id courier.MsgID) courier.Msg { m.ID_ = id; return m }

// WithUUID can be used to set the id on a msg in a chained call
func (m *DBMsg) WithUUID(uuid courier.MsgUUID) courier.Msg { m.UUID_ = uuid; return m }

// WithMetadata can be used to add metadata to a Msg
func (m *DBMsg) WithMetadata(metadata json.RawMessage) courier.Msg { m.Metadata_ = metadata; return m }

// WithFlow can be used to add flow to a Msg
func (m *DBMsg) WithFlow(flow *courier.FlowReference) courier.Msg { m.Flow_ = flow; return m }

// WithAttachment can be used to append to the media urls for a message
func (m *DBMsg) WithAttachment(url string) courier.Msg {
	m.Attachments_ = append(m.Attachments_, url)
	return m
}

func (m *DBMsg) WithLocale(lc courier.Locale) courier.Msg { m.Locale_ = null.String(lc); return m }

// WithURNAuth can be used to add a URN auth setting to a message
func (m *DBMsg) WithURNAuth(auth string) courier.Msg {
	m.URNAuth_ = auth
	return m
}
