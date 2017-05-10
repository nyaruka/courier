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

// Msg is a message which has been received by a handler
type Msg interface {
	UUID() MsgUUID
	Channel() ChannelUUID
	URN() URN
	Name() string
	Text() string
	ExternalID() string
	Date() time.Time
	MediaURLs() []string

	WithName(string) Msg
	WithExternalID(string) Msg
	WithDate(time.Time) Msg

	AddMediaURL(string)

	Release()
}

// MsgStatusUpdate is a queued item describing a status update of a message in the database
type MsgStatusUpdate interface {
	Channel() ChannelUUID
	ID() string
	ExternalID() string
	Status() MsgStatus
	Release()
}

// NewMsgUUID creates a new unique message UUID
func NewMsgUUID() MsgUUID {
	return MsgUUID{uuid.NewV4()}
}

// NewMsg creates a new message from the given params
func NewMsg(channel Channel, urn URN, text string) Msg {
	m := msgPool.Get().(*msg)
	m.clear()

	m.UUID_ = NewMsgUUID()
	m.Channel_ = channel.UUID()
	m.URN_ = urn
	m.Text_ = text
	m.Date_ = time.Now()

	return m
}

// NewMsgFromJSON creates a new message from the given JSON
func NewMsgFromJSON(msgJSON string) (Msg, error) {
	m := msgPool.Get().(*msg)
	m.clear()

	err := json.Unmarshal([]byte(msgJSON), m)
	if err != nil {
		m.Release()
		return nil, err
	}

	return m, err
}

// NewStatusUpdateForJSON creates a new status update from the given JSON
func NewStatusUpdateForJSON(statusJSON string) (MsgStatusUpdate, error) {
	s := statusPool.Get().(*msgStatusUpdate)
	s.clear()

	err := json.Unmarshal([]byte(statusJSON), s)
	if err != nil {
		s.Release()
		return nil, err
	}

	return s, err
}

// NewStatusUpdateForID creates a new status update for a message identified by its primary key
func NewStatusUpdateForID(channel Channel, id string, status MsgStatus) MsgStatusUpdate {
	s := statusPool.Get().(*msgStatusUpdate)
	s.Channel_ = channel.UUID()
	s.ID_ = id
	s.ExternalID_ = ""
	s.Status_ = status
	s.ModifiedOn_ = time.Now()
	return s
}

// NewStatusUpdateForExternalID creates a new status update for a message identified by its external ID
func NewStatusUpdateForExternalID(channel Channel, externalID string, status MsgStatus) MsgStatusUpdate {
	s := statusPool.Get().(*msgStatusUpdate)
	s.Channel_ = channel.UUID()
	s.ID_ = ""
	s.ExternalID_ = externalID
	s.Status_ = status
	s.ModifiedOn_ = time.Now()
	return s
}

var ErrNoMsg = errors.New("no message")
var ErrMsgNotFound = errors.New("message not found")

// queueMsg creates a message given the passed in arguments, returning the uuid of the created message
func queueMsg(s *server, m Msg) error {
	// if we have media, go download it to S3
	for i, mediaURL := range m.MediaURLs() {
		if strings.HasPrefix(mediaURL, "http") {
			url, err := downloadMediaToS3(s, m.UUID(), mediaURL)
			if err != nil {
				return err
			}
			m.(*msg).MediaURLs_[i] = url
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
func queueMsgStatus(s *server, status MsgStatusUpdate) error {
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

func writeMsgToRedis(s *server, m Msg, msgJSON []byte) error {
	// write it to redis
	r := s.redisPool.Get()
	defer r.Close()

	// we push to two different queues, one that is URN specific and the other that is our global queue (and to this only the URN)
	r.Send("MULTI")
	r.Send("RPUSH", fmt.Sprintf("c:u:%s", m.URN()), msgJSON)
	r.Send("RPUSH", "c:msgs", m.URN())
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

func checkMsgExists(s *server, status MsgStatusUpdate) (err error) {
	var id int64

	if status.ID() != "" {
		err = s.db.QueryRow(selectMsgIDForID, status.ID(), status.Channel()).Scan(&id)
	} else if status.ExternalID() != "" {
		err = s.db.QueryRow(selectMsgIDForExternalID, status.ExternalID(), status.Channel()).Scan(&id)
	} else {
		return fmt.Errorf("no id or external id for status update")
	}

	if err == sql.ErrNoRows {
		return ErrMsgNotFound
	}
	return err
}

func writeMsgStatusToRedis(s *server, status MsgStatusUpdate, statusJSON []byte) (err error) {
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

var msgPool = sync.Pool{New: func() interface{} { return &msg{} }}
var statusPool = sync.Pool{New: func() interface{} { return &msgStatusUpdate{} }}

//-----------------------------------------------------------------------------
// Msg implementation
//-----------------------------------------------------------------------------

type msg struct {
	UUID_       MsgUUID     `json:"uuid"`
	Channel_    ChannelUUID `json:"channel"`
	URN_        URN         `json:"urn"`
	Name_       string      `json:"name"`
	Text_       string      `json:"text"`
	ExternalID_ string      `json:"external_id"`
	Date_       time.Time   `json:"date"`
	MediaURLs_  []string    `json:"media_urls"`
}

func (m *msg) UUID() MsgUUID        { return m.UUID_ }
func (m *msg) Channel() ChannelUUID { return m.Channel_ }
func (m *msg) URN() URN             { return m.URN_ }
func (m *msg) Name() string         { return m.Name_ }
func (m *msg) Text() string         { return m.Text_ }
func (m *msg) ExternalID() string   { return m.ExternalID_ }
func (m *msg) Date() time.Time      { return m.Date_ }
func (m *msg) MediaURLs() []string  { return m.MediaURLs_ }
func (m *msg) Release()             { msgPool.Put(m) }

func (m *msg) WithName(name string) Msg     { m.Name_ = name; return m }
func (m *msg) WithDate(date time.Time) Msg  { m.Date_ = date; return m }
func (m *msg) WithExternalID(id string) Msg { m.ExternalID_ = id; return m }

func (m *msg) AddMediaURL(url string) { m.MediaURLs_ = append(m.MediaURLs_, url) }

func (m *msg) clear() {
	m.UUID_ = NilMsgUUID
	m.Channel_ = NilChannelUUID
	m.URN_ = NilURN
	m.Name_ = ""
	m.Text_ = ""
	m.ExternalID_ = ""
	m.Date_ = time.Time{}
	m.MediaURLs_ = nil
}

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

type msgStatusUpdate struct {
	Channel_    ChannelUUID `json:"channel"                  db:"channel"`
	ID_         string      `json:"id,omitempty"             db:"id"`
	ExternalID_ string      `json:"external_id,omitempty"    db:"external_id"`
	Status_     MsgStatus   `json:"status"                   db:"status"`
	ModifiedOn_ time.Time   `json:"modified_on"              db:"modified_on"`
}

func (m *msgStatusUpdate) Channel() ChannelUUID { return m.Channel_ }
func (m *msgStatusUpdate) ID() string           { return m.ID_ }
func (m *msgStatusUpdate) ExternalID() string   { return m.ExternalID_ }
func (m *msgStatusUpdate) Status() MsgStatus    { return m.Status_ }
func (m *msgStatusUpdate) Release()             { statusPool.Put(m) }

func (m *msgStatusUpdate) clear() {
	m.Channel_ = NilChannelUUID
	m.ID_ = ""
	m.ExternalID_ = ""
	m.Status_ = ""
	m.ModifiedOn_ = time.Time{}
}
