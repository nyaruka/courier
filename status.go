package courier

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

// queueMsgStatus writes the passed in status to the database, queueing it to our spool in case the database is down
func queueMsgStatus(s *server, status *MsgStatusUpdate) error {
	// first check if this msg exists
	err := checkMsgExists(s, status)
	if err == ErrMsgNotFound {
		return err
	}

	err = writeMsgStatusToRedis(s, status)

	// failed writing, write to our spool instead
	if err != nil {
		err = writeToSpool(s, "statuses", status)
	}

	return err
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

func writeMsgStatusToRedis(s *server, status *MsgStatusUpdate) (err error) {
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return err
	}

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

func (s *server) statusSpoolWalker(dir string) filepath.WalkFunc {
	return s.newSpoolWalker(dir, func(filename string, contents []byte) error {
		status, err := NewStatusUpdateForJSON(string(contents))
		if err != nil {
			log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
			os.Rename(filename, fmt.Sprintf("%s.error", filename))
			return nil
		}

		// try to flush to redis
		return writeMsgStatusToRedis(s, status)
	})
}

var statusPool = sync.Pool{New: func() interface{} { return &MsgStatusUpdate{} }}

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
