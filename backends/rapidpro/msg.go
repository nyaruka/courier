package rapidpro

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
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

// MsgPriority is the priority of our message
type MsgPriority int

// Possible values for MsgPriority
const (
	BulkPriority    MsgPriority = 100
	DefaultPriority MsgPriority = 500
	HighPriority    MsgPriority = 1000
)

// MsgVisibility is the visibility of a message
type MsgVisibility string

// Possible values for MsgVisibility
const (
	MsgVisible  MsgVisibility = "V"
	MsgDeleted  MsgVisibility = "D"
	MsgArchived MsgVisibility = "A"
)

// WriteMsg creates a message given the passed in arguments, returning the uuid of the created message
func writeMsg(b *backend, msg *courier.Msg) error {
	m := newDBMsgFromMsg(msg)

	// if we have media, go download it to S3
	for i, attachment := range msg.Attachments {
		if strings.HasPrefix(attachment, "http") {
			url, err := downloadMediaToS3(b, msg.UUID, attachment)
			if err != nil {
				return err
			}
			msg.Attachments[i] = url
		}
	}

	// try to write it our db
	err := writeMsgToDB(b, m)

	// fail? spool for later
	if err != nil {
		return courier.WriteToSpool(b.config.SpoolDir, "msgs", m)
	}

	// finally try to add this message to our handling queue
	err = addToHandleQueue(b, m)

	// set the id on the message returned (could be 0, that's ok)
	msg.ID = m.ID

	// TODO: spool backdown for failure to add to redis
	return err
}

func newDBMsgFromMsg(m *courier.Msg) *DBMsg {
	attachments := make([]string, len(m.Attachments))
	for i := range m.Attachments {
		attachments[i] = m.Attachments[i]
	}

	now := time.Now()

	rpChannel := m.Channel.(*DBChannel)

	return &DBMsg{
		OrgID:       rpChannel.OrgID(),
		UUID:        m.UUID,
		Direction:   MsgIncoming,
		Status:      courier.MsgPending,
		Visibility:  MsgVisible,
		Priority:    DefaultPriority,
		Text:        m.Text,
		Attachments: attachments,
		ExternalID:  m.ExternalID,

		ChannelID:   rpChannel.ID(),
		ChannelUUID: m.Channel.UUID(),
		URN:         m.URN,
		ContactName: m.ContactName,

		MessageCount: 1,

		NextAttempt: now,
		CreatedOn:   now,
		ModifiedOn:  now,
		QueuedOn:    now,
		SentOn:      m.ReceivedOn,
	}
}

func addToHandleQueue(b *backend, m *DBMsg) error {
	// write it to redis
	r := b.redisPool.Get()
	defer r.Close()

	// we push to two different queues, one that is URN specific and the other that is our global queue (and to this only the URN)
	r.Send("MULTI")
	r.Send("RPUSH", fmt.Sprintf("c:u:%s", m.URN), m.ID)
	r.Send("RPUSH", "c:msgs", m.URN)
	_, err := r.Do("EXEC")
	if err != nil {
		return err
	}

	return nil
}

const insertMsgSQL = `
INSERT INTO msgs_msg(org_id, direction, has_template_error, text, msg_count, error_count, priority, status, 
                     visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, queued_on, sent_on)
              VALUES(:org_id, :direction, FALSE, :text, :msg_count, :error_count, :priority, :status, 
                     :visibility, :external_id, :channel_id, :contact_id, :contact_urn_id, :created_on, :modified_on, :next_attempt, :queued_on, :sent_on)
RETURNING id
`

func writeMsgToDB(b *backend, m *DBMsg) error {
	// grab the contact for this msg
	contact, err := contactForURN(b.db, m.OrgID, m.ChannelID, m.URN, m.ContactName)

	// our db is down, write to the spool, we will write/queue this later
	if err != nil {
		return courier.WriteToSpool(b.config.SpoolDir, "msgs", m)
	}

	// set our contact and urn ids from our contact
	m.ContactID = contact.ID
	m.ContactURNID = contact.URNID

	rows, err := b.db.NamedQuery(insertMsgSQL, m)
	if err != nil {
		return err
	}
	rows.Next()
	err = rows.Scan(&m.ID)
	if err != nil {
		return err
	}
	return err
}

const selectMsgSQL = `
SELECT org_id, direction, text, msg_count, error_count, priority, status, 
       visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, queued_on, sent_on
FROM msgs_msg
WHERE id = $1
`

func readMsgFromDB(b *backend, id courier.MsgID) (*DBMsg, error) {
	m := &DBMsg{}
	err := b.db.Get(m, selectMsgSQL, id)
	return m, err
}

//-----------------------------------------------------------------------------
// Media download and classification
//-----------------------------------------------------------------------------

func downloadMediaToS3(b *backend, msgUUID courier.MsgUUID, mediaURL string) (string, error) {
	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}

	// first fetch our media
	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return "", err
	}
	resp, body, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", err
	}

	// figure out the type of our media, our mime matcher only needs ~300 bytes
	mimeType, err := filetype.Match(body[:300])
	if err != nil || mimeType == filetype.Unknown {
		mimeType = filetype.GetType(filepath.Ext(parsedURL.Path))
	}

	// we can't guess our media type, use what was provided by our response
	extension := mimeType.Extension
	if mimeType == filetype.Unknown {
		mimeType = filetype.NewType(resp.Header.Get("Content-Type"), "")
		extension = filepath.Ext(parsedURL.Path)
	}

	// create our filename
	filename := fmt.Sprintf("%s.%s", msgUUID, extension)
	path := filepath.Join(b.config.S3MediaPrefix, filename[:4], filename)
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}

	s3URL, err := utils.PutS3File(b.s3Client, b.config.S3MediaBucket, path, mimeType.MIME.Value, body)
	if err != nil {
		return "", err
	}

	// return our new media URL, which is prefixed by our content type
	return fmt.Sprintf("%s:%s", mimeType.MIME.Value, s3URL), nil
}

//-----------------------------------------------------------------------------
// Msg flusher for flushing failed writes
//-----------------------------------------------------------------------------

func (b *backend) flushMsgFile(filename string, contents []byte) error {
	msg := &DBMsg{}
	err := json.Unmarshal(contents, msg)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to write it our db
	err = writeMsgToDB(b, msg)

	// fail? oh well, we'll try again later
	if err != nil {
		return err
	}

	// finally try to add this message to our handling queue
	// TODO: if we fail here how do we avoid double inserts above?
	return addToHandleQueue(b, msg)
}

// DBMsg is our base struct to represent msgs both in our JSON and db representations
type DBMsg struct {
	OrgID       OrgID             `json:"org_id"       db:"org_id"`
	ID          courier.MsgID     `json:"id"           db:"id"`
	UUID        courier.MsgUUID   `json:"uuid"`
	Direction   MsgDirection      `json:"direction"    db:"direction"`
	Status      courier.MsgStatus `json:"status"       db:"status"`
	Visibility  MsgVisibility     `json:"visibility"   db:"visibility"`
	Priority    MsgPriority       `json:"priority"     db:"priority"`
	URN         courier.URN       `json:"urn"`
	Text        string            `json:"text"         db:"text"`
	Attachments []string          `json:"attachments"`
	ExternalID  string            `json:"external_id"  db:"external_id"`

	ChannelID    ChannelID    `json:"channel_id"      db:"channel_id"`
	ContactID    ContactID    `json:"contact_id"      db:"contact_id"`
	ContactURNID ContactURNID `json:"contact_urn_id"  db:"contact_urn_id"`

	MessageCount int `json:"msg_count"    db:"msg_count"`
	ErrorCount   int `json:"error_count"  db:"error_count"`

	ChannelUUID courier.ChannelUUID `json:"channel_uuid"`
	ContactName string              `json:"contact_name"`

	NextAttempt time.Time `json:"next_attempt"  db:"next_attempt"`
	CreatedOn   time.Time `json:"created_on"    db:"created_on"`
	ModifiedOn  time.Time `json:"modified_on"   db:"modified_on"`
	QueuedOn    time.Time `json:"queued_on"     db:"queued_on"`
	SentOn      time.Time `json:"sent_on"       db:"sent_on"`
}
