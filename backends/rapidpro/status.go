package rapidpro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
)

// newMsgStatus creates a new DBMsgStatus for the passed in parameters
func newMsgStatus(channel courier.Channel, id courier.MsgID, externalID string, status courier.MsgStatusValue, clog *courier.ChannelLog) *DBMsgStatus {
	dbChannel := channel.(*DBChannel)

	return &DBMsgStatus{
		ChannelUUID_: channel.UUID(),
		ChannelID_:   dbChannel.ID(),
		ID_:          id,
		OldURN_:      urns.NilURN,
		NewURN_:      urns.NilURN,
		ExternalID_:  externalID,
		Status_:      status,
		ModifiedOn_:  time.Now().In(time.UTC),
		LogUUID:      clog.UUID(),
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

// the craziness below lets us update our status to 'F' and schedule retries without knowing anything about the message
const sqlUpdateMsgByID = `
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
	failed_reason = CASE
		WHEN
			error_count >= 2
		THEN
			'E'
		ELSE
			failed_reason
	    END,
	sent_on = CASE 
		WHEN
			s.status IN ('W', 'S', 'D')
		THEN
			COALESCE(sent_on, NOW())
		ELSE
			NULL
		END,
	external_id = CASE
		WHEN 
			s.external_id != ''
		THEN
			s.external_id
		ELSE
			msgs_msg.external_id
		END,
	modified_on = NOW(),
	log_uuids = array_append(log_uuids, s.log_uuid::uuid)
FROM
	(VALUES(:msg_id, :channel_id, :status, :external_id, :log_uuid)) 
AS 
	s(msg_id, channel_id, status, external_id, log_uuid) 
WHERE 
	msgs_msg.id = s.msg_id::bigint AND
	msgs_msg.channel_id = s.channel_id::int AND 
	msgs_msg.direction = 'O'
`

const sqlUpdateMsgByExternalID = `
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
	failed_reason = CASE
		WHEN
			error_count >= 2
		THEN
			'E'
		ELSE
			failed_reason
	    END,
	sent_on = CASE 
		WHEN
			:status IN ('W', 'S', 'D')
		THEN
			COALESCE(sent_on, NOW())
		ELSE
			NULL
		END,
	modified_on = :modified_on,
	log_uuids = array_append(log_uuids, :log_uuid)
WHERE 
	msgs_msg.id = (SELECT msgs_msg.id FROM msgs_msg WHERE msgs_msg.external_id = :external_id AND msgs_msg.channel_id = :channel_id AND msgs_msg.direction = 'O' LIMIT 1)
RETURNING 
	msgs_msg.id
`

// writeMsgStatusToDB writes the passed in msg status to our db
func writeMsgStatusToDB(ctx context.Context, b *backend, status *DBMsgStatus) error {
	if status.ID() == courier.NilMsgID && status.ExternalID() == "" {
		return fmt.Errorf("attempt to update msg status without id or external id")
	}

	var rows *sqlx.Rows
	var err error

	if status.ID() != courier.NilMsgID {
		err = dbutil.BulkQuery(context.Background(), b.db, sqlUpdateMsgByID, []*DBMsgStatus{status})
		return err
	}

	rows, err = b.db.NamedQueryContext(ctx, sqlUpdateMsgByExternalID, status)
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

//-----------------------------------------------------------------------------
// MsgStatusUpdate implementation
//-----------------------------------------------------------------------------

// DBMsgStatus represents a status update on a message
type DBMsgStatus struct {
	ChannelUUID_ courier.ChannelUUID    `json:"channel_uuid"             db:"channel_uuid"`
	ChannelID_   courier.ChannelID      `json:"channel_id"               db:"channel_id"`
	ID_          courier.MsgID          `json:"msg_id,omitempty"         db:"msg_id"`
	OldURN_      urns.URN               `json:"old_urn"                  db:"old_urn"`
	NewURN_      urns.URN               `json:"new_urn"                  db:"new_urn"`
	ExternalID_  string                 `json:"external_id,omitempty"    db:"external_id"`
	Status_      courier.MsgStatusValue `json:"status"                   db:"status"`
	ModifiedOn_  time.Time              `json:"modified_on"              db:"modified_on"`
	LogUUID      courier.ChannelLogUUID `json:"log_uuid"                 db:"log_uuid"`
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

func (s *DBMsgStatus) SetUpdatedURN(old, new urns.URN) error {
	// check by nil URN
	if old == urns.NilURN || new == urns.NilURN {
		return errors.New("cannot update contact URN from/to nil URN")
	}
	// only update to the same scheme
	if old.Scheme() != new.Scheme() {
		return errors.New("cannot update contact URN to a different scheme")
	}
	// don't update to the same URN path
	if old.Path() == new.Path() {
		return errors.New("cannot update contact URN to the same path")
	}
	s.OldURN_ = old
	s.NewURN_ = new
	return nil
}
func (s *DBMsgStatus) UpdatedURN() (urns.URN, urns.URN) {
	return s.OldURN_, s.NewURN_
}
func (s *DBMsgStatus) HasUpdatedURN() bool {
	if s.OldURN_ != urns.NilURN && s.NewURN_ != urns.NilURN {
		return true
	}
	return false
}

func (s *DBMsgStatus) ExternalID() string      { return s.ExternalID_ }
func (s *DBMsgStatus) SetExternalID(id string) { s.ExternalID_ = id }

func (s *DBMsgStatus) Status() courier.MsgStatusValue          { return s.Status_ }
func (s *DBMsgStatus) SetStatus(status courier.MsgStatusValue) { s.Status_ = status }

type StatusWriter struct {
	*syncx.Batcher[*DBMsgStatus]
}

func NewStatusWriter(db *sqlx.DB, spoolDir string, wg *sync.WaitGroup) *StatusWriter {
	return &StatusWriter{
		Batcher: syncx.NewBatcher[*DBMsgStatus](func(batch []*DBMsgStatus) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeMsgStatuses(ctx, db, spoolDir, batch)
		}, time.Millisecond*500, 1000, wg),
	}
}

func writeMsgStatuses(ctx context.Context, db *sqlx.DB, spoolDir string, statuses []*DBMsgStatus) {
	for _, batch := range utils.ChunkSlice(statuses, 1000) {
		err := dbutil.BulkQuery(ctx, db, sqlUpdateMsgByID, batch)

		// if we received an error, try again one at a time (in case it is one value hanging us up)
		if err != nil {
			for _, s := range batch {
				err = dbutil.BulkQuery(ctx, db, sqlUpdateMsgByID, []*DBMsgStatus{s})
				if err != nil {
					log := logrus.WithField("comp", "status writer").WithField("msg_id", s.ID())

					if qerr := dbutil.AsQueryError(err); qerr != nil {
						query, params := qerr.Query()
						log = log.WithFields(logrus.Fields{"sql": query, "sql_params": params})
					}

					log.WithError(err).Error("error writing msg status")

					err = courier.WriteToSpool(spoolDir, "statuses", s)
					if err != nil {
						logrus.WithField("comp", "status committer").WithError(err).Error("error writing status to spool")
					}
				}
			}
		}
	}
}
