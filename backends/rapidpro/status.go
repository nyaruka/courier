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
	// create our msg status object
	dbStatus := &DBMsgStatus{
		ChannelUUID: status.Channel.UUID(),
		ID:          status.ID,
		ExternalID:  status.ExternalID,
		Status:      status.Status,
		ModifiedOn:  status.CreatedOn,
	}

	err := writeMsgStatusToDB(b, dbStatus)
	if err == courier.ErrMsgNotFound {
		return err
	}

	// failed writing, write to our spool instead
	if err != nil {
		err = courier.WriteToSpool(b.config.SpoolDir, "statuses", dbStatus)
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

const updateMsgID = `
UPDATE msgs_msg SET status = :status, modified_on = :modified_on WHERE msgs_msg.id IN
	(SELECT msgs_msg.id FROM msgs_msg INNER JOIN channels_channel ON (msgs_msg.channel_id = channels_channel.id) 
WHERE (msgs_msg.id = :msg_id AND channels_channel.uuid = :channel_uuid))
`

const updateMsgExternalID = `
UPDATE msgs_msg SET status = :status, modified_on = :modified_on WHERE msgs_msg.id IN
	(SELECT msgs_msg.id FROM msgs_msg INNER JOIN channels_channel ON (msgs_msg.channel_id = channels_channel.id) 
WHERE (msgs_msg.external_id = :external_id AND channels_channel.uuid = :channel_uuid))
`

// writeMsgStatusToDB writes the passed in msg status to our db
func writeMsgStatusToDB(b *backend, status *DBMsgStatus) error {
	var result sql.Result
	var err error
	if status.ID != courier.NilMsgID {
		result, err = b.db.NamedExec(updateMsgID, status)
	} else if status.ExternalID != "" {
		result, err = b.db.NamedExec(updateMsgExternalID, status)
	} else {
		return fmt.Errorf("attempt to update msg status without id or external id")
	}
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return courier.ErrMsgNotFound
	}

	return nil
}

func (b *backend) flushStatusFile(filename string, contents []byte) error {
	status := &DBMsgStatus{}
	err := json.Unmarshal(contents, status)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to flush to redis
	return writeMsgStatusToDB(b, status)
}

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// DBMsgStatus represents a status update on a message
type DBMsgStatus struct {
	ChannelUUID courier.ChannelUUID `json:"channel_uuid"             db:"channel_uuid"`
	ID          courier.MsgID       `json:"msg_id,omitempty"             db:"msg_id"`
	ExternalID  string              `json:"external_id,omitempty"    db:"external_id"`
	Status      courier.MsgStatus   `json:"status"                   db:"status"`
	ModifiedOn  time.Time           `json:"modified_on"              db:"modified_on"`
}
