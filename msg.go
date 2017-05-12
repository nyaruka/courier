package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"net/http"
	"net/url"

	"github.com/nyaruka/courier/utils"
	uuid "github.com/satori/go.uuid"
	"gopkg.in/h2non/filetype.v1"
)

// ErrMsgNotFound is returned when trying to queue the status for a Msg that doesn't exit
var ErrMsgNotFound = errors.New("message not found")

// MsgID is our typing of the db int type
type MsgID int64

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID(0)

// MsgStatus is the status of a message
type MsgStatus string

// Possible values for MsgStatus
const (
	MsgPending   MsgStatus = "P"
	MsgQueued    MsgStatus = "Q"
	MsgSent      MsgStatus = "S"
	MsgDelivered MsgStatus = "D"
	MsgFailed    MsgStatus = "F"
	NilMsgStatus MsgStatus = ""
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
	MsgVisible       MsgVisibility = "V"
	MsgDeleted       MsgVisibility = "D"
	MsgArchived      MsgVisibility = "A"
	NilMsgVisibility MsgVisibility = ""
)

// MsgUUID is the UUID of a message which has been received
type MsgUUID struct {
	uuid.UUID
}

// NilMsgUUID is a "zero value" message UUID
var NilMsgUUID = MsgUUID{uuid.Nil}

// NewMsgUUID creates a new unique message UUID
func NewMsgUUID() MsgUUID {
	return MsgUUID{uuid.NewV4()}
}

// NewIncomingMsg creates a new message from the given params
func NewIncomingMsg(channel *Channel, urn URN, text string) *Msg {
	m := msgPool.Get().(*Msg)
	m.clear()

	m.UUID = NewMsgUUID()
	m.OrgID = channel.OrgID
	m.ChannelID = channel.ID
	m.ChannelUUID = channel.UUID
	m.ContactURN = urn
	m.Text = text

	m.Visibility = MsgVisible
	m.MessageCount = 1
	m.Direction = MsgIncoming
	m.Status = MsgPending
	m.Priority = DefaultPriority

	now := time.Now()
	m.CreatedOn = now
	m.ModifiedOn = now
	m.SentOn = now
	m.QueuedOn = now
	m.NextAttempt = now

	return m
}

// NewMsgFromJSON creates a new message from the given JSON
func NewMsgFromJSON(msgJSON string) (*Msg, error) {
	m := msgPool.Get().(*Msg)
	m.clear()

	err := json.Unmarshal([]byte(msgJSON), m)
	if err != nil {
		m.Release()
		return nil, err
	}

	return m, err
}

// queueMsg creates a message given the passed in arguments, returning the uuid of the created message
func queueMsg(s *server, m *Msg) error {
	// if we have media, go download it to S3
	for i, mediaURL := range m.MediaURLs {
		if strings.HasPrefix(mediaURL, "http") {
			url, err := downloadMediaToS3(s, m.UUID, mediaURL)
			if err != nil {
				return err
			}
			m.MediaURLs[i] = url
		}
	}

	// try to write it our db
	err := writeMsgToDB(s, m)

	// fail? spool for later
	if err != nil {
		return writeToSpool(s, "msgs", m)
	}

	// finally try to add this message to our handling queue
	err = addToHandleQueue(s, m)

	// TODO: spool backdown for failure to add to redis
	return err
}

func addToHandleQueue(s *server, m *Msg) error {
	// write it to redis
	r := s.redisPool.Get()
	defer r.Close()

	// we push to two different queues, one that is URN specific and the other that is our global queue (and to this only the URN)
	r.Send("MULTI")
	r.Send("RPUSH", fmt.Sprintf("c:u:%s", m.ContactURN), m.ID)
	r.Send("RPUSH", "c:msgs", m.ContactURN)
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

func writeMsgToDB(s *server, m *Msg) error {
	// grab the contact for this msg
	contact, err := contactForURN(s.db, m.OrgID, m.ChannelID, m.ContactURN, m.ContactName)

	// our db is down, write to the spool, we will write/queue this later
	if err != nil {
		return writeToSpool(s, "msgs", m)
	}

	// set our contact and urn ids from our contact
	m.ContactID = contact.ID
	m.ContactURNID = contact.URNID

	rows, err := s.db.NamedQuery(insertMsgSQL, m)
	if err != nil {
		return err
	}
	if rows.Next() {
		rows.Scan(&m.ID)
	}
	return err
}

//-----------------------------------------------------------------------------
// Media download and classification
//-----------------------------------------------------------------------------

func downloadMediaToS3(s *server, msgUUID MsgUUID, mediaURL string) (string, error) {
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
	s3URL, err := putS3File(s, filename, mimeType.MIME.Value, body)
	if err != nil {
		return "", err
	}

	// return our new media URL, which is prefixed by our content type
	return fmt.Sprintf("%s:%s", mimeType.MIME.Value, s3URL), nil
}

//-----------------------------------------------------------------------------
// Spool walker for flushing failed writes
//-----------------------------------------------------------------------------

func (s *server) msgSpoolWalker(dir string) filepath.WalkFunc {
	return s.newSpoolWalker(dir, func(filename string, contents []byte) error {
		msg, err := NewMsgFromJSON(string(contents))
		if err != nil {
			log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
			os.Rename(filename, fmt.Sprintf("%s.error", filename))
			return nil
		}

		// try to write it our db
		err = writeMsgToDB(s, msg)

		// fail? oh well, we'll try again later
		if err != nil {
			return err
		}

		// finally try to add this message to our handling queue
		// TODO: if we fail here how do we avoid double inserts above?
		return addToHandleQueue(s, msg)
	})
}

//-----------------------------------------------------------------------------
// Msg
//-----------------------------------------------------------------------------

// Msg is our base struct to represent msgs both in our JSON and db representations
type Msg struct {
	OrgID      OrgID         `json:"org_id"       db:"org_id"`
	ID         MsgID         `json:"id"           db:"id"`
	UUID       MsgUUID       `json:"uuid"`
	Direction  MsgDirection  `json:"direction"    db:"direction"`
	Status     MsgStatus     `json:"status"       db:"status"`
	Visibility MsgVisibility `json:"visibility"   db:"visibility"`
	Priority   MsgPriority   `json:"priority"     db:"priority"`
	Text       string        `json:"text"         db:"text"`
	MediaURLs  []string      `json:"media_urls"`
	ExternalID string        `json:"external_id"  db:"external_id"`

	ChannelID    ChannelID    `json:"channel_id"      db:"channel_id"`
	ContactID    ContactID    `json:"contact_id"      db:"contact_id"`
	ContactURNID ContactURNID `json:"contact_urn_id"  db:"contact_urn_id"`

	MessageCount int `json:"msg_count"    db:"msg_count"`
	ErrorCount   int `json:"error_count"  db:"error_count"`

	ChannelUUID ChannelUUID `json:"channel_uuid"`
	ContactURN  URN         `json:"urn"`
	ContactName string      `json:"contact_name"`

	NextAttempt time.Time `json:"next_attempt"  db:"next_attempt"`
	CreatedOn   time.Time `json:"created_on"    db:"created_on"`
	ModifiedOn  time.Time `json:"modified_on"   db:"modified_on"`
	QueuedOn    time.Time `json:"queued_on"     db:"queued_on"`
	SentOn      time.Time `json:"sent_on"       db:"sent_on"`
}

// Release releases this msg and assigns it back to our pool for reuse
func (m *Msg) Release() { msgPool.Put(m) }

// WithContactName can be used to set the contact name on a msg
func (m *Msg) WithContactName(name string) *Msg { m.ContactName = name; return m }

// WithReceivedOn can be used to set sent_on on a msg in a chained call
func (m *Msg) WithReceivedOn(date time.Time) *Msg { m.SentOn = date; return m }

// WithExternalID can be used to set the external id on a msg in a chained call
func (m *Msg) WithExternalID(id string) *Msg { m.ExternalID = id; return m }

// AddMediaURL can be used to append to the media urls for a message
func (m *Msg) AddMediaURL(url string) { m.MediaURLs = append(m.MediaURLs, url) }

// clears our message for future reuse in our message pool
func (m *Msg) clear() {
	m.OrgID = NilOrgID
	m.ID = NilMsgID
	m.UUID = NilMsgUUID
	m.Direction = NilMsgDirection
	m.Status = NilMsgStatus
	m.Text = ""
	m.Priority = DefaultPriority
	m.Visibility = NilMsgVisibility
	m.MediaURLs = nil
	m.ExternalID = ""

	m.ChannelID = NilChannelID
	m.ContactID = NilContactID
	m.ContactURNID = NilContactURNID

	m.MessageCount = 0
	m.ErrorCount = 0

	m.ChannelUUID = NilChannelUUID
	m.ContactURN = NilURN
	m.ContactName = ""

	m.NextAttempt = time.Time{}
	m.CreatedOn = time.Time{}
	m.ModifiedOn = time.Time{}
	m.QueuedOn = time.Time{}
	m.SentOn = time.Time{}
}

var msgPool = sync.Pool{New: func() interface{} { return &Msg{} }}
