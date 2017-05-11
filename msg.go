package courier

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
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
type MsgID struct {
	sql.NullInt64
}

// NilMsgID is our nil value for MsgID
var NilMsgID = MsgID{sql.NullInt64{Int64: 0, Valid: false}}

// MsgStatus is the status of a message
type MsgStatus string

// Possible values for MsgStatus
const (
	MsgPending   MsgStatus = "P"
	MsgQueued    MsgStatus = "Q"
	MsgSent      MsgStatus = "S"
	MsgDelivered MsgStatus = "D"
	MsgFailed    MsgStatus = "F"
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

// NewMsg creates a new message from the given params
func NewMsg(channel *Channel, urn URN, text string) *Msg {
	m := msgPool.Get().(*Msg)
	m.clear()

	m.UUID = NewMsgUUID()
	m.ChannelUUID = channel.UUID
	m.ContactURN = urn
	m.Text = text
	m.SentOn = time.Now()

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

// NewStatusUpdateForJSON creates a new status update from the given JSON
func NewStatusUpdateForJSON(statusJSON string) (*MsgStatusUpdate, error) {
	s := statusPool.Get().(*MsgStatusUpdate)
	s.clear()

	err := json.Unmarshal([]byte(statusJSON), s)
	if err != nil {
		s.Release()
		return nil, err
	}

	return s, err
}

// NewStatusUpdateForID creates a new status update for a message identified by its primary key
func NewStatusUpdateForID(channel *Channel, id string, status MsgStatus) *MsgStatusUpdate {
	s := statusPool.Get().(*MsgStatusUpdate)
	s.ChannelUUID = channel.UUID
	s.ID = id
	s.ExternalID = ""
	s.Status = status
	s.ModifiedOn = time.Now()
	return s
}

// NewStatusUpdateForExternalID creates a new status update for a message identified by its external ID
func NewStatusUpdateForExternalID(channel *Channel, externalID string, status MsgStatus) *MsgStatusUpdate {
	s := statusPool.Get().(*MsgStatusUpdate)
	s.ChannelUUID = channel.UUID
	s.ID = ""
	s.ExternalID = externalID
	s.Status = status
	s.ModifiedOn = time.Now()
	return s
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

	// marshal this msg to JSON
	msgJSON, err := json.Marshal(m)
	if err != nil {
		return err
	}

	// try to write this to redis
	err = writeMsgToRedis(s, m, msgJSON)

	// we failed our write to redis, write to disk instead
	if err != nil {
		err = writeToSpool(s, "msgs", msgJSON)
	}

	return err
}

// queueMsgStatus writes the passed in status to the database, queueing it to our spool in case the database is down
func queueMsgStatus(s *server, status *MsgStatusUpdate) error {
	// first check if this msg exists
	err := checkMsgExists(s, status)
	if err == ErrMsgNotFound {
		return err
	}

	// other errors are DB errors, courier keeps running in this case and queues up the status anyways
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return err
	}

	err = writeMsgStatusToRedis(s, status, statusJSON)

	// failed writing, write to our spool instead
	if err != nil {
		err = writeToSpool(s, "statuses", statusJSON)
	}

	return err
}

func startMsgSpoolFlusher(s *server) {
	s.waitGroup.Add(1)
	defer s.waitGroup.Done()

	msgsDir := path.Join(s.config.Spool_Dir, "msgs")
	statusesDir := path.Join(s.config.Spool_Dir, "statuses")

	msgWalker := s.msgSpoolWalker(msgsDir)
	statusWalker := s.statusSpoolWalker(statusesDir)

	log.Println("[X] Spool: flush process started")

	// runs until stopped, checking every 30 seconds if there is anything to flush from our spool
	for {
		select {

		// our server is shutting down, exit
		case <-s.stopChan:
			log.Println("[X] Spool: flush process stopped")
			return

		// every 30 seconds we check to see if there are any files to spool
		case <-time.After(30 * time.Second):
			filepath.Walk(msgsDir, msgWalker)
			filepath.Walk(statusesDir, statusWalker)
		}
	}

}

func writeMsgToRedis(s *server, m *Msg, msgJSON []byte) error {
	// write it to redis
	r := s.redisPool.Get()
	defer r.Close()

	// we push to two different queues, one that is URN specific and the other that is our global queue (and to this only the URN)
	r.Send("MULTI")
	r.Send("RPUSH", fmt.Sprintf("c:u:%s", m.ContactURN), msgJSON)
	r.Send("RPUSH", "c:msgs", m.ContactURN)
	_, err := r.Do("EXEC")
	if err != nil {
		return err
	}

	return nil
}

const selectMsgIDForID = `
SELECT m."id" FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id") WHERE (m."id" = $1 AND c."uuid" = $2)`

const selectMsgIDForExternalID = `
SELECT m."id" FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id") WHERE (m."external_id" = $1 AND c."uuid" = $2)`

func checkMsgExists(s *server, status *MsgStatusUpdate) (err error) {
	var id int64

	if status.ID != "" {
		err = s.db.QueryRow(selectMsgIDForID, status.ID, status.ChannelUUID).Scan(&id)
	} else if status.ExternalID != "" {
		err = s.db.QueryRow(selectMsgIDForExternalID, status.ExternalID, status.ChannelUUID).Scan(&id)
	} else {
		return fmt.Errorf("no id or external id for status update")
	}

	if err == sql.ErrNoRows {
		return ErrMsgNotFound
	}
	return err
}

func writeMsgStatusToRedis(s *server, status *MsgStatusUpdate, statusJSON []byte) (err error) {
	// write it to redis
	r := s.redisPool.Get()
	defer r.Close()

	// we push status updates to a single redis queue called c:statuses
	_, err = r.Do("RPUSH", "c:statuses", statusJSON)
	if err != nil {
		return err
	}

	return nil
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
// Spool Utility functions
//-----------------------------------------------------------------------------

func testSpoolDirs(s *server) (err error) {
	msgsDir := path.Join(s.config.Spool_Dir, "msgs")
	if _, err = os.Stat(msgsDir); os.IsNotExist(err) {
		err = os.MkdirAll(msgsDir, 0770)
	}
	if err != nil {
		return err
	}

	statusesDir := path.Join(s.config.Spool_Dir, "statuses")
	if _, err = os.Stat(statusesDir); os.IsNotExist(err) {
		err = os.MkdirAll(statusesDir, 0770)
	}
	return err
}

func writeToSpool(s *server, subdir string, contents []byte) error {
	filename := path.Join(s.config.Spool_Dir, subdir, fmt.Sprintf("%d.json", time.Now().UnixNano()))
	return ioutil.WriteFile(filename, contents, 0640)
}

type fileFlusher func(filename string, contents []byte) error

func (s *server) newSpoolWalker(dir string, flusher fileFlusher) filepath.WalkFunc {
	return func(filename string, info os.FileInfo, err error) error {
		if filename == dir {
			return nil
		}

		// we've been stopped, exit
		if s.stopped {
			return errors.New("spool flush process stopped")
		}

		// we don't care about subdirectories
		if info.IsDir() {
			return filepath.SkipDir
		}

		// ignore non-json files
		if !strings.HasSuffix(filename, ".json") {
			return nil
		}

		// otherwise, read our msg json
		contents, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("ERROR reading spool file '%s': %s\n", filename, err)
			return nil
		}

		err = flusher(filename, contents)
		if err != nil {
			log.Printf("ERROR flushing file '%s': %s\n", filename, err)
			return err
		}

		log.Printf("Spool: flushed '%s' to redis", filename)

		// we flushed to redis, remove our file if it is still present
		if _, e := os.Stat(filename); e == nil {
			err = os.Remove(filename)
		}
		return err
	}
}

func (s *server) msgSpoolWalker(dir string) filepath.WalkFunc {
	return s.newSpoolWalker(dir, func(filename string, contents []byte) error {
		msg, err := NewMsgFromJSON(string(contents))
		if err != nil {
			log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
			os.Rename(filename, fmt.Sprintf("%s.error", filename))
			return nil
		}

		// try to flush to redis
		return writeMsgToRedis(s, msg, contents)
	})
}

func (s *server) statusSpoolWalker(dir string) filepath.WalkFunc {
	return s.newSpoolWalker(dir, func(filename string, contents []byte) error {
		status, err := NewStatusUpdateForJSON(string(contents))
		if err != nil {
			log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
			os.Rename(filename, fmt.Sprintf("%s.error", filename))
			return nil
		}

		// try to flush to redis
		return writeMsgStatusToRedis(s, status, contents)
	})
}

var msgPool = sync.Pool{New: func() interface{} { return &Msg{} }}
var statusPool = sync.Pool{New: func() interface{} { return &MsgStatusUpdate{} }}

//-----------------------------------------------------------------------------
// Msg implementation
//-----------------------------------------------------------------------------

// Msg is our base struct to represent msgs both in our JSON and db representations
type Msg struct {
	OrgID      OrgID    `json:"org_id"       db:"org_id"`
	ID         MsgID    `json:"id"           db:"id"`
	UUID       MsgUUID  `json:"uuid"`
	Direction  string   `json:"direction"    db:"direction"`
	Text       string   `json:"text"         db:"text"`
	Priority   int      `json:"priority"     db:"priority"`
	Visibility string   `json:"visibility"   db:"visibility"`
	MediaURLs  []string `json:"media_urls"`
	ExternalID string   `json:"external_id"  db:"external_id"`

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
	m.Direction = ""
	m.Text = ""
	m.Priority = 0
	m.Visibility = ""
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

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// MsgStatusUpdate represents a status update on a message
type MsgStatusUpdate struct {
	ChannelUUID ChannelUUID `json:"channel"                  db:"channel"`
	ID          string      `json:"id,omitempty"             db:"id"`
	ExternalID  string      `json:"external_id,omitempty"    db:"external_id"`
	Status      MsgStatus   `json:"status"                   db:"status"`
	ModifiedOn  time.Time   `json:"modified_on"              db:"modified_on"`
}

// Release releases this status and assigns it back to our pool for reuse
func (m *MsgStatusUpdate) Release() { statusPool.Put(m) }

func (m *MsgStatusUpdate) clear() {
	m.ChannelUUID = NilChannelUUID
	m.ID = ""
	m.ExternalID = ""
	m.Status = ""
	m.ModifiedOn = time.Time{}
}
