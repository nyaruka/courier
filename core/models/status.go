package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/core/events"
	"github.com/nyaruka/null/v3"
)

// StatusUpdate represents a status update on a message
type StatusUpdate struct {
	ChannelUUID_        ChannelUUID  `json:"channel_uuid"                    db:"channel_uuid"`
	ChannelID_          ChannelID    `json:"channel_id"                      db:"channel_id"`
	MsgUUID_            MsgUUID      `json:"msg_uuid,omitempty"              db:"msg_uuid"`
	ExternalIdentifier_ string       `json:"external_identifier,omitempty"   db:"external_identifier"`
	Status_             MsgStatus    `json:"status"                          db:"status"`
	LogUUID             svclogs.UUID `json:"log_uuid"                        db:"log_uuid"`
}

func (s *StatusUpdate) EventUUID() uuids.UUID    { return uuids.UUID(s.MsgUUID_) }
func (s *StatusUpdate) ChannelUUID() ChannelUUID { return s.ChannelUUID_ }
func (s *StatusUpdate) MsgUUID() MsgUUID         { return s.MsgUUID_ }

func (s *StatusUpdate) ExternalIdentifier() string      { return s.ExternalIdentifier_ }
func (s *StatusUpdate) SetExternalIdentifier(id string) { s.ExternalIdentifier_ = id }

func (s *StatusUpdate) Status() MsgStatus          { return s.Status_ }
func (s *StatusUpdate) SetStatus(status MsgStatus) { s.Status_ = status }

// NewStatusUpdate creates a new status update for the given message UUID
func NewStatusUpdate(channel *Channel, uuid MsgUUID, status MsgStatus, clog *ChannelLog) *StatusUpdate {
	return &StatusUpdate{
		ChannelUUID_: channel.UUID(),
		ChannelID_:   channel.ID(),
		MsgUUID_:     uuid,
		Status_:      status,
		LogUUID:      clog.UUID,
	}
}

// NewStatusUpdateByExternalID creates a new status update for the given external id
func NewStatusUpdateByExternalID(channel *Channel, externalID string, status MsgStatus, clog *ChannelLog) *StatusUpdate {
	return &StatusUpdate{
		ChannelUUID_:        channel.UUID(),
		ChannelID_:          channel.ID(),
		ExternalIdentifier_: externalID,
		Status_:             status,
		LogUUID:             clog.UUID,
	}
}

// WriteStatusUpdate queues the passed in status update to be written to the database by the batched status writer
func WriteStatusUpdate(ctx context.Context, rt *runtime.Runtime, status *StatusUpdate) error {
	log := slog.With("msg_uuid", status.MsgUUID(), "msg_external_id", status.ExternalIdentifier(), "status", status.Status())

	if status.MsgUUID() == "" && status.ExternalIdentifier() == "" {
		return errors.New("message status with no UUID or external id")
	}

	if status.MsgUUID() != "" {
		// this is a message we've just sent and were given an external id for
		if status.ExternalIdentifier() != "" {
			rc := rt.VK.Get()
			defer rc.Close()

			err := sentExternalIDs.Set(ctx, rc, fmt.Sprintf("%d|%s", status.ChannelID_, status.ExternalIdentifier_), string(status.MsgUUID()))
			if err != nil {
				log.Error("error recording external id", "error", err)
			}
		}

		// we sent a message that errored so clear our sent flag to allow it to be retried
		if status.Status() == MsgStatusErrored {
			err := ClearMsgSent(ctx, rt, status.MsgUUID())
			if err != nil {
				log.Error("error clearing sent flags", "error", err)
			}
		}
	}

	// queue the status to written by the batch writer
	statusWriter.Queue(status)
	log.Debug("status update queued")

	return nil
}

// StatusWriter handles batched writes of status updates to the database
type StatusWriter struct {
	*syncx.Batcher[*StatusUpdate]
}

// NewStatusWriter creates a new status update writer
func NewStatusWriter(rt *runtime.Runtime) *StatusWriter {
	return &StatusWriter{
		Batcher: syncx.NewBatcher(func(batch []*StatusUpdate) {
			// keep this comfortably under the shutdown watchdog since final flushes happen during shutdown -
			// statuses that can't be written in time will be spooled instead
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			writeStatusUpdates(ctx, rt, batch)

		}, 1000, time.Millisecond*500, 1000),
	}
}

// flushStatuses is the flush function for the status spool - it retries writing spooled status updates to the
// database, returning those that fail again so they're respooled
func flushStatuses(ctx context.Context, rt *runtime.Runtime, batch []*StatusUpdate) ([]*StatusUpdate, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	unresolved, err := writeStatusUpdatesToDB(ctx, rt, batch)
	if err != nil {
		return nil, err // no partial success from a batched update so retry the whole batch later
	}
	for _, s := range unresolved {
		slog.Warn(fmt.Sprintf("unable to find message with channel_id=%d and external_identifier=%s", s.ChannelID_, s.ExternalIdentifier_))
	}
	return nil, nil
}

// tries to write a batch of message statuses to the database and spools those that fail
func writeStatusUpdates(ctx context.Context, rt *runtime.Runtime, batch []*StatusUpdate) {
	log := slog.With("comp", "status writer")

	unresolved, err := writeStatusUpdatesToDB(ctx, rt, batch)

	// if we received an error, try again one at a time (in case it is one value hanging us up)
	if err != nil {
		var toSpool []*StatusUpdate

		for _, s := range batch {
			_, err = writeStatusUpdatesToDB(ctx, rt, []*StatusUpdate{s})
			if err != nil {
				log := log.With("msg_uuid", s.MsgUUID())

				if qerr := dbutil.AsQueryError(err); qerr != nil {
					query, params := qerr.Query()
					log = log.With("sql", query, "sql_params", params)
				}

				log.Error("error writing msg status", "error", err)

				toSpool = append(toSpool, s)
			}
		}

		if len(toSpool) > 0 {
			if err := statusSpool.Add(toSpool); err != nil {
				log.Error("error writing statuses to spool", "error", err) // just have to log and move on
			}
		}
	} else {
		for _, s := range unresolved {
			log.Warn(fmt.Sprintf("unable to find message with channel_id=%d and external_identifier=%s", s.ChannelID_, s.ExternalIdentifier_))
		}
	}
}

// writes a batch of msg status updates to the database - messages that can't be resolved are returned and aren't
// considered an error
func writeStatusUpdatesToDB(ctx context.Context, rt *runtime.Runtime, statuses []*StatusUpdate) ([]*StatusUpdate, error) {
	// get the statuses which are missing msg UUIDs
	missingUUID := make([]*StatusUpdate, 0, len(statuses))
	for _, s := range statuses {
		if s.MsgUUID_ == "" {
			missingUUID = append(missingUUID, s)
		}
	}

	if len(missingUUID) > 0 {
		if err := resolveStatusUpdateByExternalIdentifier(ctx, rt, missingUUID); err != nil {
			return nil, fmt.Errorf("error resolving status updates by external identifier: %w", err)
		}
	}

	resolved := make([]*StatusUpdate, 0, len(statuses))
	unresolved := make([]*StatusUpdate, 0, len(statuses))

	for _, s := range statuses {
		if s.MsgUUID_ != "" {
			resolved = append(resolved, s)
		} else {
			unresolved = append(unresolved, s)
		}
	}

	if len(resolved) > 0 {
		changes, err := WriteStatusUpdates(ctx, rt, resolved)
		if err != nil {
			return nil, fmt.Errorf("error writing resolved status updates: %w", err)
		}

		for _, c := range changes {
			if _, err := rt.Writers.History.Queue(c); err != nil {
				slog.Error("error queuing status change to writer", "error", err, "msg_uuid", c.MsgUUID, "msg_status", c.MsgStatus)
			}
		}

		// publish changes to any subscribed history sockets - best-effort with its own short timeout so a Centrifugo
		// or Valkey stall can't eat into the batch's DB write budget
		pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := PublishStatusChanges(pubCtx, rt, changes); err != nil {
			slog.Error("error publishing status changes", "error", err)
		}
	}

	return unresolved, nil
}

const sqlResolveStatusByExternalIdentifier = `
SELECT uuid, channel_id, external_identifier
  FROM msgs_msg
 WHERE (channel_id, external_identifier) IN (VALUES(:channel_id::int, :external_identifier))`

// tries to resolve msg UUIDs for the given statuses using their external identifiers
func resolveStatusUpdateByExternalIdentifier(ctx context.Context, rt *runtime.Runtime, statuses []*StatusUpdate) error {
	rc := rt.VK.Get()
	defer rc.Close()

	chAndExtKeys := make([]string, len(statuses))
	for i, s := range statuses {
		chAndExtKeys[i] = fmt.Sprintf("%d|%s", s.ChannelID_, s.ExternalIdentifier_)
	}
	cachedUUIDs, err := sentExternalIDs.MGet(ctx, rc, chAndExtKeys...)
	if err != nil {
		// log error but we continue and try to get ids from the database
		slog.Error("error looking up sent message ids in valkey", "error", err)
	}

	// collect the statuses that couldn't be resolved from cache, update the ones that could
	notInCache := make([]*StatusUpdate, 0, len(statuses))
	for i, val := range cachedUUIDs {
		if val != "" && uuids.Is(val) {
			statuses[i].MsgUUID_ = MsgUUID(val)
		} else {
			notInCache = append(notInCache, statuses[i])
		}
	}

	if len(notInCache) == 0 {
		return nil
	}

	// create a mapping of channel id + external id -> status
	type ext struct {
		channelID  ChannelID
		externalID string
	}
	statusesByExt := make(map[ext]*StatusUpdate, len(notInCache))
	for _, s := range statuses {
		statusesByExt[ext{s.ChannelID_, s.ExternalIdentifier_}] = s
	}

	sql, params, err := dbutil.BulkSQL(rt.DB, sqlResolveStatusByExternalIdentifier, notInCache)
	if err != nil {
		return err
	}

	rows, err := rt.DB.QueryContext(ctx, sql, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var msgUUID MsgUUID
	var channelID ChannelID
	var externalID string

	for rows.Next() {
		if err := rows.Scan(&msgUUID, &channelID, &externalID); err != nil {
			return fmt.Errorf("error scanning rows: %w", err)
		}

		// find the status with this channel ID and external ID and update its msg UUID
		s := statusesByExt[ext{channelID, externalID}]
		s.MsgUUID_ = msgUUID
	}

	return rows.Err()
}

// the craziness below lets us update our status to 'F' and schedule retries without knowing anything about the message
const sqlUpdateMsgByUUID = `
UPDATE msgs_msg SET 
	status = CASE 
		WHEN s.status = 'E' 
		THEN CASE WHEN error_count >= 2 OR msgs_msg.status = 'F' THEN 'F' ELSE 'E' END 
		ELSE s.status 
		END,
	error_count = CASE WHEN s.status = 'E' THEN error_count + 1 ELSE error_count END,
	next_attempt = CASE WHEN s.status = 'E' THEN NOW() + (5 * (error_count+1) * interval '1 minutes') ELSE next_attempt END,
	failed_reason = CASE WHEN error_count >= 2 THEN 'E' ELSE failed_reason END,
	sent_on = CASE WHEN s.status IN ('W', 'S', 'D', 'R') THEN COALESCE(sent_on, NOW()) ELSE NULL END,
	external_identifier = CASE WHEN s.external_identifier != '' THEN s.external_identifier ELSE msgs_msg.external_identifier END,
	modified_on = NOW(),
	log_uuids = array_append(log_uuids, s.log_uuid)
    FROM 
        (VALUES(:msg_uuid::uuid, :channel_id::int, :status, :external_identifier, :log_uuid::uuid)) AS s(msg_uuid, channel_id, status, external_identifier, log_uuid),
        contacts_contact c
    WHERE msgs_msg.uuid = s.msg_uuid AND msgs_msg.channel_id = s.channel_id AND msgs_msg.direction = 'O' AND c.id = msgs_msg.contact_id
RETURNING msgs_msg.uuid AS msg_uuid, msgs_msg.status AS msg_status, msgs_msg.failed_reason, c.uuid AS contact_uuid, msgs_msg.org_id`

func WriteStatusUpdates(ctx context.Context, rt *runtime.Runtime, statuses []*StatusUpdate) ([]*StatusChange, error) {
	// rewrite query as a bulk operation
	query, args, err := dbutil.BulkSQL(rt.DB, sqlUpdateMsgByUUID, statuses)
	if err != nil {
		return nil, err
	}

	rows, err := rt.DB.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error writing statuses in bulk: %w", err)
	}
	defer rows.Close()

	changes := make([]*StatusChange, 0, len(statuses))

	for rows.Next() {
		sc := &StatusChange{CreatedOn: time.Now()}
		if err := rows.StructScan(&sc); err != nil {
			return nil, fmt.Errorf("error scanning status change: %w", err)
		}

		changes = append(changes, sc)
	}

	return changes, nil
}

// StatusChange represents an actual change in status for a message
type StatusChange struct {
	ContactUUID  ContactUUID `db:"contact_uuid"`
	MsgUUID      MsgUUID     `db:"msg_uuid"`
	MsgStatus    MsgStatus   `db:"msg_status"`
	FailedReason null.String `db:"failed_reason"`
	OrgID        OrgID       `db:"org_id"`
	CreatedOn    time.Time
}

func (s *StatusChange) DynamoKey() dynamo.Key {
	return dynamo.Key{PK: fmt.Sprintf("con#%s", s.ContactUUID), SK: fmt.Sprintf("evt#%s#sts", s.MsgUUID)}
}

func (s *StatusChange) MarshalDynamo() (*dynamo.Item, error) {
	data := map[string]any{
		"created_on": s.CreatedOn,
		"status":     statusNames[s.MsgStatus],
	}
	if reason := s.reason(); reason != "" {
		data["reason"] = reason
	}

	return &dynamo.Item{Key: s.DynamoKey(), OrgID: int(s.OrgID), Data: data}, nil
}

// the client facing names of the statuses, as used in both history items and published events
var statusNames = map[MsgStatus]string{
	MsgStatusWired:     "wired",
	MsgStatusSent:      "sent",
	MsgStatusDelivered: "delivered",
	MsgStatusRead:      "read",
	MsgStatusErrored:   "errored",
	MsgStatusFailed:    "failed",
}

// the client facing reason for this change, or "" if there isn't one
func (s *StatusChange) reason() string {
	if s.MsgStatus == MsgStatusFailed && s.FailedReason == "E" {
		return "error_limit"
	}
	return ""
}

// historyEvent renders this change as the msg_status_changed event published to the contact's history socket
func (s *StatusChange) historyEvent() *events.MsgStatusChanged {
	e := events.NewMsgStatusChanged(events.EventUUID(s.MsgUUID), statusNames[s.MsgStatus], s.reason())
	e.CreatedOn_ = s.CreatedOn
	return e
}
