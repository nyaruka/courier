package rapidpro

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mime"

	"github.com/garyburd/redigo/redis"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
	null "gopkg.in/guregu/null.v3"
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
func writeMsg(b *backend, msg courier.Msg) error {
	m := msg.(*DBMsg)

	// this msg has already been written (we received it twice), we are a no op
	if m.AlreadyWritten_ {
		return nil
	}

	// if we have media, go download it to S3
	for i, attachment := range m.Attachments_ {
		if strings.HasPrefix(attachment, "http") {
			url, err := downloadMediaToS3(b, m.OrgID_, m.UUID_, attachment)
			if err != nil {
				return err
			}
			m.Attachments_[i] = url
		}
	}

	// try to write it our db
	err := writeMsgToDB(b, m)

	// fail? spool for later
	if err != nil {
		logrus.WithError(err).WithField("msg", m.UUID().String()).Error("error writing to db")
		return courier.WriteToSpool(b.config.SpoolDir, "msgs", m)
	}

	// mark this msg as having been seen
	writeMsgSeen(b, m)
	return err
}

// newMsg creates a new DBMsg object with the passed in parameters
func newMsg(direction MsgDirection, channel courier.Channel, urn urns.URN, text string) *DBMsg {
	now := time.Now()
	dbChannel := channel.(*DBChannel)

	return &DBMsg{
		OrgID_:        dbChannel.OrgID(),
		UUID_:         courier.NewMsgUUID(),
		Direction_:    direction,
		Status_:       courier.MsgPending,
		Visibility_:   MsgVisible,
		HighPriority_: null.NewBool(false, false),
		Text_:         text,

		ChannelID_:   dbChannel.ID(),
		ChannelUUID_: dbChannel.UUID(),

		URN_:          urn,
		MessageCount_: 1,

		NextAttempt_: now,
		CreatedOn_:   now,
		ModifiedOn_:  now,
		QueuedOn_:    now,

		Channel_:        channel,
		WorkerToken_:    "",
		AlreadyWritten_: false,
	}
}

const insertMsgSQL = `
INSERT INTO msgs_msg(org_id, direction, text, attachments, msg_count, error_count, high_priority, status, 
                     visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, queued_on, sent_on)
              VALUES(:org_id, :direction, :text, :attachments, :msg_count, :error_count, :high_priority, :status, 
                     :visibility, :external_id, :channel_id, :contact_id, :contact_urn_id, :created_on, :modified_on, :next_attempt, :queued_on, :sent_on)
RETURNING id
`

func writeMsgToDB(b *backend, m *DBMsg) error {
	// grab the contact for this msg
	contact, err := contactForURN(b.db, m.OrgID_, m.ChannelID_, m.URN_, m.ContactName_)

	// our db is down, write to the spool, we will write/queue this later
	if err != nil {
		return err
	}

	// set our contact and urn ids from our contact
	m.ContactID_ = contact.ID
	m.ContactURNID_ = contact.URNID

	rows, err := b.db.NamedQuery(insertMsgSQL, m)
	if err != nil {
		return err
	}
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&m.ID_)
	if err != nil {
		return err
	}

	// queue this up to be handled by RapidPro
	rc := b.redisPool.Get()
	defer rc.Close()
	err = queueMsgHandling(rc, m.OrgID_, m.ContactID_, m.ID_, contact.IsNew)

	// if we had a problem queueing the handling, log it, but our message is written, it'll
	// get picked up by our rapidpro catch-all after a period
	if err != nil {
		logrus.WithError(err).WithField("msg_id", m.ID_.Int64).Error("error queueing msg handling")
	}

	return nil
}

const selectMsgSQL = `
SELECT org_id, direction, text, attachments, msg_count, error_count, high_priority, status, 
       visibility, external_id, channel_id, contact_id, contact_urn_id, created_on, modified_on, next_attempt, queued_on, sent_on
FROM msgs_msg
WHERE id = $1
`

func readMsgFromDB(b *backend, id courier.MsgID) (*DBMsg, error) {
	m := &DBMsg{
		ID_: id,
	}
	err := b.db.Get(m, selectMsgSQL, id)
	return m, err
}

//-----------------------------------------------------------------------------
// Media download and classification
//-----------------------------------------------------------------------------

func downloadMediaToS3(b *backend, orgID OrgID, msgUUID courier.MsgUUID, mediaURL string) (string, error) {
	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		return "", err
	}

	// first fetch our media
	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := utils.GetHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", err
	}

	mimeType := ""
	extension := filepath.Ext(parsedURL.Path)
	if extension != "" {
		extension = extension[1:]
	}

	// first try getting our mime type from the first 300 bytes of our body
	fileType, err := filetype.Match(body[:300])
	if fileType != filetype.Unknown {
		mimeType = fileType.MIME.Value
		extension = fileType.Extension
	} else {
		// if that didn't work, try from our extension
		fileType = filetype.GetType(extension)
		if fileType != filetype.Unknown {
			mimeType = fileType.MIME.Value
			extension = fileType.Extension
		}
	}

	// we still don't know our mime type, use our content header instead
	if mimeType == "" {
		mimeType, _, _ = mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if extension == "" {
			extensions, err := mime.ExtensionsByType(mimeType)
			if extensions == nil || err != nil {
				extension = ""
			} else {
				extension = extensions[0][1:]
			}
		}
	}

	// create our filename
	filename := msgUUID.String()
	if extension != "" {
		filename = fmt.Sprintf("%s.%s", msgUUID, extension)
	}
	path := filepath.Join(b.config.S3MediaPrefix, strconv.FormatInt(orgID.Int64, 10), filename[:4], filename[4:8], filename)
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}

	s3URL, err := utils.PutS3File(b.s3Client, b.config.S3MediaBucket, path, mimeType, body)
	if err != nil {
		return "", err
	}

	// return our new media URL, which is prefixed by our content type
	return fmt.Sprintf("%s:%s", mimeType, s3URL), nil
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
	return err
}

//-----------------------------------------------------------------------------
// Deduping utility methods
//-----------------------------------------------------------------------------

var luaMsgSeen = redis.NewScript(3, `-- KEYS: [Window, PrevWindow, Fingerprint]
	-- try to look up in window
	local uuid = redis.call("hget", KEYS[1], KEYS[3])

	-- didn't find it, try in our previous window
	if not uuid then 
		uuid = redis.call("hget", KEYS[2], KEYS[3])
	end
	
	-- return the uuid found if any
	return uuid
`)

// checkMsgSeen tries to look up whether a msg with the fingerprint passed in was seen in window or prevWindow. If
// found returns the UUID of that msg, if not returns empty string
func checkMsgSeen(b *backend, msg *DBMsg) courier.MsgUUID {
	r := b.redisPool.Get()
	defer r.Close()

	fingerprint := msg.fingerprint()

	now := time.Now().In(time.UTC)
	prev := now.Add(time.Second * -2)
	windowKey := fmt.Sprintf("seen:msgs:%s:%02d", now.Format("2006-01-02-15:04"), now.Second()/2*2)
	prevWindowKey := fmt.Sprintf("seen:msgs:%s:%02d", prev.Format("2006-01-02-15:04"), prev.Second()/2*2)

	// try to look up our UUID from either window or prev window
	foundUUID, _ := redis.String(luaMsgSeen.Do(r, windowKey, prevWindowKey, fingerprint))
	if foundUUID != "" {
		return courier.NewMsgUUIDFromString(foundUUID)
	}
	return courier.NilMsgUUID
}

var luaWriteMsgSeen = redis.NewScript(3, `-- KEYS: [Window, Fingerprint, UUID]
	redis.call("hset", KEYS[1], KEYS[2], KEYS[3])
	redis.call("pexpire", KEYS[1], 5000)
`)

// writeMsgSeen writes that the message with the passed in fingerprint and UUID was seen in the
// passed in window
func writeMsgSeen(b *backend, msg *DBMsg) {
	r := b.redisPool.Get()
	defer r.Close()

	fingerprint := msg.fingerprint()
	now := time.Now().In(time.UTC)
	windowKey := fmt.Sprintf("seen:msgs:%s:%02d", now.Format("2006-01-02-15:04"), now.Second()/2*2)

	luaWriteMsgSeen.Do(r, windowKey, fingerprint, msg.UUID().String())
}

//-----------------------------------------------------------------------------
// Our implementation of Msg interface
//-----------------------------------------------------------------------------

// DBMsg is our base struct to represent msgs both in our JSON and db representations
type DBMsg struct {
	OrgID_        OrgID                  `json:"org_id"        db:"org_id"`
	ID_           courier.MsgID          `json:"id"            db:"id"`
	UUID_         courier.MsgUUID        `json:"uuid"`
	Direction_    MsgDirection           `json:"direction"     db:"direction"`
	Status_       courier.MsgStatusValue `json:"status"        db:"status"`
	Visibility_   MsgVisibility          `json:"visibility"    db:"visibility"`
	HighPriority_ null.Bool              `json:"high_priority" db:"high_priority"`
	URN_          urns.URN               `json:"urn"`
	Text_         string                 `json:"text"          db:"text"`
	Attachments_  pq.StringArray         `json:"attachments"   db:"attachments"`
	ExternalID_   null.String            `json:"external_id"   db:"external_id"`

	ChannelID_    courier.ChannelID `json:"channel_id"      db:"channel_id"`
	ContactID_    ContactID         `json:"contact_id"      db:"contact_id"`
	ContactURNID_ ContactURNID      `json:"contact_urn_id"  db:"contact_urn_id"`

	MessageCount_ int `json:"msg_count"    db:"msg_count"`
	ErrorCount_   int `json:"error_count"  db:"error_count"`

	ChannelUUID_ courier.ChannelUUID `json:"channel_uuid"`
	ContactName_ string              `json:"contact_name"`

	NextAttempt_ time.Time `json:"next_attempt"  db:"next_attempt"`
	CreatedOn_   time.Time `json:"created_on"    db:"created_on"`
	ModifiedOn_  time.Time `json:"modified_on"   db:"modified_on"`
	QueuedOn_    time.Time `json:"queued_on"     db:"queued_on"`
	SentOn_      time.Time `json:"sent_on"       db:"sent_on"`

	Channel_        courier.Channel   `json:"-"`
	WorkerToken_    queue.WorkerToken `json:"-"`
	AlreadyWritten_ bool              `json:"-"`
}

func (m *DBMsg) Channel() courier.Channel { return m.Channel_ }
func (m *DBMsg) ID() courier.MsgID        { return m.ID_ }
func (m *DBMsg) ReceiveID() int64         { return m.ID_.Int64 }
func (m *DBMsg) UUID() courier.MsgUUID    { return m.UUID_ }
func (m *DBMsg) Text() string             { return m.Text_ }
func (m *DBMsg) Attachments() []string    { return []string(m.Attachments_) }
func (m *DBMsg) ExternalID() string       { return m.ExternalID_.String }
func (m *DBMsg) URN() urns.URN            { return m.URN_ }
func (m *DBMsg) ContactName() string      { return m.ContactName_ }
func (m *DBMsg) HighPriority() bool       { return m.HighPriority_.Valid && m.HighPriority_.Bool }

func (m *DBMsg) ReceivedOn() *time.Time { return &m.SentOn_ }
func (m *DBMsg) SentOn() *time.Time     { return &m.SentOn_ }

// fingerprint returns a fingerprint for this msg, suitable for figuring out if this is a dupe
func (m *DBMsg) fingerprint() string {
	return fmt.Sprintf("%s:%s:%s", m.Channel_.UUID(), m.URN_, m.Text_)
}

// WithContactName can be used to set the contact name on a msg
func (m *DBMsg) WithContactName(name string) courier.Msg { m.ContactName_ = name; return m }

// WithReceivedOn can be used to set sent_on on a msg in a chained call
func (m *DBMsg) WithReceivedOn(date time.Time) courier.Msg { m.SentOn_ = date; return m }

// WithExternalID can be used to set the external id on a msg in a chained call
func (m *DBMsg) WithExternalID(id string) courier.Msg { m.ExternalID_ = null.StringFrom(id); return m }

// WithID can be used to set the id on a msg in a chained call
func (m *DBMsg) WithID(id courier.MsgID) courier.Msg { m.ID_ = id; return m }

// WithUUID can be used to set the id on a msg in a chained call
func (m *DBMsg) WithUUID(uuid courier.MsgUUID) courier.Msg { m.UUID_ = uuid; return m }

// WithAttachment can be used to append to the media urls for a message
func (m *DBMsg) WithAttachment(url string) courier.Msg {
	m.Attachments_ = append(m.Attachments_, url)
	return m
}
