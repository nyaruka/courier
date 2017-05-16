package rapidpro

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nyaruka/courier"
)

// WriteMsgStatus writes the passed in status to the database, queueing it to our spool in case the database is down
func writeMsgStatus(b *backend, status *courier.MsgStatusUpdate) error {
	// first check if this msg exists
	err := checkMsgExists(b, status)
	if err == courier.ErrMsgNotFound {
		return err
	}

	// if so, create our local msg status object
	rpStatus := &MsgStatusUpdate{
		ChannelUUID: status.Channel.UUID(),
		ID:          status.ID,
		ExternalID:  status.ExternalID,
		Status:      status.Status,
		ModifiedOn:  status.CreatedOn,
	}

	err = writeMsgStatusToRedis(b, rpStatus)

	// failed writing, write to our spool instead
	if err != nil {
		err = courier.WriteToSpool(b.config.SpoolDir, "statuses", rpStatus)
	}

	return err
}

const selectMsgIDForID = `
SELECT m."id" FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id") WHERE (m."id" = $1 AND c."uuid" = $2)`

const selectMsgIDForExternalID = `
SELECT m."id" FROM "msgs_msg" m INNER JOIN "channels_channel" c ON (m."channel_id" = c."id") WHERE (m."external_id" = $1 AND c."uuid" = $2)`

func checkMsgExists(b *backend, status *courier.MsgStatusUpdate) (err error) {
	var id int64

	if status.ID != courier.NilMsgID {
		err = b.db.QueryRow(selectMsgIDForID, status.ID, status.Channel.UUID()).Scan(&id)
	} else if status.ExternalID != "" {
		err = b.db.QueryRow(selectMsgIDForExternalID, status.ExternalID, status.Channel.UUID()).Scan(&id)
	} else {
		return fmt.Errorf("no id or external id for status update")
	}

	if err == sql.ErrNoRows {
		return courier.ErrMsgNotFound
	}
	return err
}

func writeMsgStatusToRedis(b *backend, status *MsgStatusUpdate) (err error) {
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return err
	}

	// write it to redis
	r := b.redisPool.Get()
	defer r.Close()

	// we push status updates to a single redis queue called c:statuses
	_, err = r.Do("RPUSH", "c:statuses", statusJSON)
	if err != nil {
		return err
	}

	return nil
}

func (b *backend) flushStatusFile(filename string, contents []byte) error {
	status := &MsgStatusUpdate{}
	err := json.Unmarshal(contents, status)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to flush to redis
	return writeMsgStatusToRedis(b, status)
}

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// MsgStatusUpdate represents a status update on a message
type MsgStatusUpdate struct {
	ChannelUUID courier.ChannelUUID `json:"channel"                  db:"channel"`
	ID          courier.MsgID       `json:"id,omitempty"             db:"id"`
	ExternalID  string              `json:"external_id,omitempty"    db:"external_id"`
	Status      courier.MsgStatus   `json:"status"                   db:"status"`
	ModifiedOn  time.Time           `json:"modified_on"              db:"modified_on"`
}
