package rapidpro

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
)

// newMsgStatus creates a new DBMsgStatus for the passed in parameters
func newMsgStatus(channel courier.Channel, id courier.MsgID, externalID string, status courier.MsgStatusValue) *DBMsgStatus {
	dbChannel := channel.(*DBChannel)

	return &DBMsgStatus{
		ChannelUUID_: channel.UUID(),
		ChannelID_:   dbChannel.ID(),
		ID_:          id,
		ExternalID_:  externalID,
		Status_:      status,
		ModifiedOn_:  time.Now().In(time.UTC),
	}
}

// writeMsgStatus writes the passed in status to the database, queueing it to our spool in case the database is down
func writeMsgStatus(ctx context.Context, b *backend, status courier.MsgStatus) error {
	dbStatus := status.(*DBMsgStatus)

	err := writeMsgStatusToDB(ctx, b, dbStatus)

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

func checkMsgExists(b *backend, status courier.MsgStatus) (err error) {
	var id int64

	if status.ID() != courier.NilMsgID {
		err = b.db.QueryRow(selectMsgIDForID, status.ID(), status.ChannelUUID()).Scan(&id)
	} else if status.ExternalID() != "" {
		err = b.db.QueryRow(selectMsgIDForExternalID, status.ExternalID(), status.ChannelUUID()).Scan(&id)
	} else {
		return fmt.Errorf("no id or external id for status update")
	}

	if err == sql.ErrNoRows {
		return courier.ErrMsgNotFound
	}
	return err
}

// the craziness below lets us update our status to 'F' and schedule retries without knowing anything about the message
const updateMsgID = `
UPDATE msgs_msg SET 
	status = CASE 
		WHEN 
			:status = 'E' 
		THEN CASE 
			WHEN 
				error_count >= 2 OR status = 'F' 
			THEN 
				'F' 
			ELSE 
				'E' 
			END 
		ELSE 
			:status 
		END,
	error_count = CASE 
		WHEN 
			:status = 'E' 
		THEN 
			error_count + 1 
		ELSE 
			error_count 
		END,
	next_attempt = CASE 
		WHEN 
			:status = 'E' 
		THEN 
			NOW() + (5 * (error_count+1) * interval '1 minutes') 
		ELSE 
			next_attempt 
		END,
	sent_on = CASE 
		WHEN 
			:status = 'W' 
		THEN 
			NOW() 
		ELSE 
			sent_on 
		END,
	external_id = CASE
		WHEN 
			:external_id != ''
		THEN
			:external_id
		ELSE
			external_id
		END,
	modified_on = :modified_on
WHERE 
	msgs_msg.id = :msg_id AND
	msgs_msg.channel_id = :channel_id
RETURNING 
	msgs_msg.id
`

const updateMsgExternalID = `
UPDATE msgs_msg SET 
	status = CASE 
		WHEN 
			:status = 'E' 
		THEN CASE 
			WHEN 
				error_count >= 2 OR status = 'F' 
			THEN 
				'F' 
			ELSE 
				'E' 
			END 
		ELSE 
			:status 
		END,
	error_count = CASE 
		WHEN 
			:status = 'E' 
		THEN 
			error_count + 1 
		ELSE 
			error_count 
		END,
	next_attempt = CASE 
		WHEN 
			:status = 'E' 
		THEN 
			NOW() + (5 * (error_count+1) * interval '1 minutes') 
		ELSE 
			next_attempt 
		END,
	sent_on = CASE 
		WHEN 
			:status = 'W' 
		THEN 
			NOW() 
		ELSE 
			sent_on 
		END,
	modified_on = :modified_on
WHERE 
	msgs_msg.id = (SELECT msgs_msg.id FROM msgs_msg WHERE msgs_msg.external_id = :external_id AND msgs_msg.channel_id = :channel_id LIMIT 1)
RETURNING 
	msgs_msg.id
`

// writeMsgStatusToDB writes the passed in msg status to our db
func writeMsgStatusToDB(ctx context.Context, b *backend, status *DBMsgStatus) error {
	var rows *sqlx.Rows
	var err error

	if status.ID() != courier.NilMsgID {
		rows, err = b.db.NamedQueryContext(ctx, updateMsgID, status)
	} else if status.ExternalID() != "" {
		rows, err = b.db.NamedQueryContext(ctx, updateMsgExternalID, status)
	} else {
		return fmt.Errorf("attempt to update msg status without id or external id")
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	// scan and read the id of the msg that was updated
	if rows.Next() {
		rows.Scan(&status.ID_)
	} else {
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

	// try to flush to our db
	err = writeMsgStatusToDB(context.Background(), b, status)

	// not finding the message is ok for status updates
	if err == courier.ErrMsgNotFound {
		return nil
	}

	// Ignore wrong status update for incoming messages
	if err == courier.ErrWrongIncomingMsgStatus {
		return nil
	}

	return err
}

const bulkUpdateMsgStatusSQL = `
UPDATE msgs_msg SET 
	status = CASE 
		WHEN 
			s.status = 'E' 
		THEN CASE 
			WHEN 
				error_count >= 2 OR msgs_msg.status = 'F' 
			THEN 
				'F' 
			ELSE 
				'E' 
			END 
		ELSE 
			s.status 
		END,
	error_count = CASE 
		WHEN 
			s.status = 'E' 
		THEN 
			error_count + 1 
		ELSE 
			error_count 
		END,
	next_attempt = CASE 
		WHEN 
			s.status = 'E' 
		THEN 
			NOW() + (5 * (error_count+1) * interval '1 minutes') 
		ELSE 
			next_attempt 
		END,
	sent_on = CASE 
		WHEN 
			s.status = 'W' 
		THEN 
			NOW() 
		ELSE 
			sent_on 
		END,
	external_id = CASE
		WHEN 
			s.external_id != ''
		THEN
			s.external_id
		ELSE
			msgs_msg.external_id
		END,
	modified_on = NOW()
FROM
	(VALUES(:msg_id, :channel_id, :status, :external_id)) 
AS 
	s(msg_id, channel_id, status, external_id) 
WHERE 
	msgs_msg.id = s.msg_id::int AND
	msgs_msg.channel_id = s.channel_id::int
RETURNING 
	msgs_msg.id
`

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// DBMsgStatus represents a status update on a message
type DBMsgStatus struct {
	ChannelUUID_ courier.ChannelUUID    `json:"channel_uuid"             db:"channel_uuid"`
	ChannelID_   courier.ChannelID      `json:"channel_id"               db:"channel_id"`
	ID_          courier.MsgID          `json:"msg_id,omitempty"         db:"msg_id"`
	ExternalID_  string                 `json:"external_id,omitempty"    db:"external_id"`
	Status_      courier.MsgStatusValue `json:"status"                   db:"status"`
	ModifiedOn_  time.Time              `json:"modified_on"              db:"modified_on"`

	logs []*courier.ChannelLog
}

func (s *DBMsgStatus) EventID() int64 { return int64(s.ID_) }

func (s *DBMsgStatus) ChannelUUID() courier.ChannelUUID { return s.ChannelUUID_ }
func (s *DBMsgStatus) ID() courier.MsgID                { return s.ID_ }

func (s *DBMsgStatus) RowID() string {
	if s.ID_ != courier.NilMsgID {
		return strconv.FormatInt(int64(s.ID_), 10)
	} else if s.ExternalID_ != "" {
		return s.ExternalID_
	}
	return ""
}

func (s *DBMsgStatus) ExternalID() string      { return s.ExternalID_ }
func (s *DBMsgStatus) SetExternalID(id string) { s.ExternalID_ = id }

func (s *DBMsgStatus) Logs() []*courier.ChannelLog    { return s.logs }
func (s *DBMsgStatus) AddLog(log *courier.ChannelLog) { s.logs = append(s.logs, log) }

func (s *DBMsgStatus) Status() courier.MsgStatusValue          { return s.Status_ }
func (s *DBMsgStatus) SetStatus(status courier.MsgStatusValue) { s.Status_ = status }
