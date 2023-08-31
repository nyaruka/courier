package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// creates a new message status update
func newStatusUpdate(channel courier.Channel, id courier.MsgID, externalID string, status courier.MsgStatus, clog *courier.ChannelLog) *StatusUpdate {
	dbChannel := channel.(*DBChannel)

	return &StatusUpdate{
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

func (b *backend) flushStatusFile(filename string, contents []byte) error {
	ctx := context.Background()
	status := &StatusUpdate{}
	err := json.Unmarshal(contents, status)
	if err != nil {
		logrus.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to flush to our db
	_, err = writeStatusUpdatesToDB(ctx, b.db, []*StatusUpdate{status})
	return err
}

//-----------------------------------------------------------------------------
// StatusUpdate implementation
//-----------------------------------------------------------------------------

// StatusUpdate represents a status update on a message
type StatusUpdate struct {
	ChannelUUID_ courier.ChannelUUID    `json:"channel_uuid"             db:"channel_uuid"`
	ChannelID_   courier.ChannelID      `json:"channel_id"               db:"channel_id"`
	ID_          courier.MsgID          `json:"msg_id,omitempty"         db:"msg_id"`
	OldURN_      urns.URN               `json:"old_urn"                  db:"old_urn"`
	NewURN_      urns.URN               `json:"new_urn"                  db:"new_urn"`
	ExternalID_  string                 `json:"external_id,omitempty"    db:"external_id"`
	Status_      courier.MsgStatus      `json:"status"                   db:"status"`
	ModifiedOn_  time.Time              `json:"modified_on"              db:"modified_on"`
	LogUUID      courier.ChannelLogUUID `json:"log_uuid"                 db:"log_uuid"`
}

func (s *StatusUpdate) EventID() int64 { return int64(s.ID_) }

func (s *StatusUpdate) ChannelUUID() courier.ChannelUUID { return s.ChannelUUID_ }
func (s *StatusUpdate) ID() courier.MsgID                { return s.ID_ }

func (s *StatusUpdate) RowID() string {
	if s.ID_ != courier.NilMsgID {
		return strconv.FormatInt(int64(s.ID_), 10)
	} else if s.ExternalID_ != "" {
		return s.ExternalID_
	}
	return ""
}

func (s *StatusUpdate) SetUpdatedURN(old, new urns.URN) error {
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
func (s *StatusUpdate) UpdatedURN() (urns.URN, urns.URN) {
	return s.OldURN_, s.NewURN_
}
func (s *StatusUpdate) HasUpdatedURN() bool {
	if s.OldURN_ != urns.NilURN && s.NewURN_ != urns.NilURN {
		return true
	}
	return false
}

func (s *StatusUpdate) ExternalID() string      { return s.ExternalID_ }
func (s *StatusUpdate) SetExternalID(id string) { s.ExternalID_ = id }

func (s *StatusUpdate) Status() courier.MsgStatus          { return s.Status_ }
func (s *StatusUpdate) SetStatus(status courier.MsgStatus) { s.Status_ = status }

type StatusWriter struct {
	*syncx.Batcher[*StatusUpdate]
}

func NewStatusWriter(db *sqlx.DB, spoolDir string, wg *sync.WaitGroup) *StatusWriter {
	return &StatusWriter{
		Batcher: syncx.NewBatcher[*StatusUpdate](func(batch []*StatusUpdate) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			writeStatuseUpdates(ctx, db, spoolDir, batch)
		}, 1000, time.Millisecond*500, 1000, wg),
	}
}

// tries to write all the message statuses to the database and spools those that fail
func writeStatuseUpdates(ctx context.Context, db *sqlx.DB, spoolDir string, statuses []*StatusUpdate) {
	log := logrus.WithField("comp", "status writer")

	for _, batch := range utils.ChunkSlice(statuses, 1000) {
		unresolved, err := writeStatusUpdatesToDB(ctx, db, batch)

		// if we received an error, try again one at a time (in case it is one value hanging us up)
		if err != nil {
			for _, s := range batch {
				_, err = writeStatusUpdatesToDB(ctx, db, []*StatusUpdate{s})
				if err != nil {
					log := log.WithField("msg_id", s.ID())

					if qerr := dbutil.AsQueryError(err); qerr != nil {
						query, params := qerr.Query()
						log = log.WithFields(logrus.Fields{"sql": query, "sql_params": params})
					}

					log.WithError(err).Error("error writing msg status")

					err := courier.WriteToSpool(spoolDir, "statuses", s)
					if err != nil {
						log.WithError(err).Error("error writing status to spool") // just have to log and move on
					}
				}
			}
		} else {
			for _, s := range unresolved {
				log.Warnf("unable to find message with channel_id=%d and external_id=%s", s.ChannelID_, s.ExternalID_)
			}
		}
	}
}

// writes a batch of msg status updates to the database - messages that can't be resolved are returned and aren't
// considered an error
func writeStatusUpdatesToDB(ctx context.Context, db *sqlx.DB, statuses []*StatusUpdate) ([]*StatusUpdate, error) {
	// get the statuses which have external ID instead of a message ID
	missingID := make([]*StatusUpdate, 0, 500)
	for _, s := range statuses {
		if s.ID_ == courier.NilMsgID {
			missingID = append(missingID, s)
		}
	}

	// try to resolve channel ID + external ID to message IDs
	if len(missingID) > 0 {
		if err := resolveStatusUpdateMsgIDs(ctx, db, missingID); err != nil {
			return nil, err
		}
	}

	resolved := make([]*StatusUpdate, 0, len(statuses))
	unresolved := make([]*StatusUpdate, 0, len(statuses))

	for _, s := range statuses {
		if s.ID_ != courier.NilMsgID {
			resolved = append(resolved, s)
		} else {
			unresolved = append(unresolved, s)
		}
	}

	err := dbutil.BulkQuery(ctx, db, sqlUpdateMsgByID, resolved)
	if err != nil {
		return nil, errors.Wrap(err, "error updating status")
	}

	return unresolved, nil
}

const sqlResolveStatusMsgIDs = `
SELECT id, channel_id, external_id 
  FROM msgs_msg 
 WHERE (channel_id, external_id) IN (VALUES(CAST(:channel_id AS int), :external_id))`

// resolveStatusUpdateMsgIDs tries to resolve msg IDs for the given statuses - if there's no matching channel id + external id pair
// found for a status, that status will be left with a nil msg ID.
func resolveStatusUpdateMsgIDs(ctx context.Context, db *sqlx.DB, statuses []*StatusUpdate) error {
	// create a mapping of channel id + external id -> status
	type ext struct {
		channelID  courier.ChannelID
		externalID string
	}
	statusesByExt := make(map[ext]*StatusUpdate, len(statuses))
	for _, s := range statuses {
		statusesByExt[ext{s.ChannelID_, s.ExternalID_}] = s
	}

	sql, params, err := dbutil.BulkSQL(db, sqlResolveStatusMsgIDs, statuses)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, sql, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var msgID courier.MsgID
	var channelID courier.ChannelID
	var externalID string

	for rows.Next() {
		if err := rows.Scan(&msgID, &channelID, &externalID); err != nil {
			return errors.Wrap(err, "error scanning rows")
		}

		// find the status with this channel ID and external ID and update its msg ID
		s := statusesByExt[ext{channelID, externalID}]
		s.ID_ = msgID
	}

	return rows.Err()
}
