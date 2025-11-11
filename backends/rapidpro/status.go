package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/syncx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

// creates a new message status update
func newStatusUpdate(channel courier.Channel, uuid models.MsgUUID, externalID string, status models.MsgStatus, clog *courier.ChannelLog) *models.StatusUpdate {
	dbChannel := channel.(*models.Channel)

	return &models.StatusUpdate{
		ChannelUUID_: channel.UUID(),
		ChannelID_:   dbChannel.ID(),
		MsgUUID_:     uuid,
		OldURN_:      urns.NilURN,
		NewURN_:      urns.NilURN,
		ExternalID_:  externalID,
		Status_:      status,
		LogUUID:      clog.UUID,
	}
}

func (b *backend) flushStatusFile(filename string, contents []byte) error {
	ctx := context.Background()
	status := &models.StatusUpdate{}
	err := json.Unmarshal(contents, status)
	if err != nil {
		slog.Info(fmt.Sprintf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err))
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to flush to our db
	_, err = b.writeStatusUpdatesToDB(ctx, []*models.StatusUpdate{status})
	return err
}

// StatusWriter handles batched writes of status updates to the database
type StatusWriter struct {
	*syncx.Batcher[*models.StatusUpdate]
}

// NewStatusWriter creates a new status update writer
func NewStatusWriter(b *backend, spoolDir string) *StatusWriter {
	return &StatusWriter{
		Batcher: syncx.NewBatcher(func(batch []*models.StatusUpdate) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			b.writeStatuseUpdates(ctx, spoolDir, batch)

		}, 1000, time.Millisecond*500, 1000),
	}
}

// tries to write a batch of message statuses to the database and spools those that fail
func (b *backend) writeStatuseUpdates(ctx context.Context, spoolDir string, batch []*models.StatusUpdate) {
	log := slog.With("comp", "status writer")

	unresolved, err := b.writeStatusUpdatesToDB(ctx, batch)

	// if we received an error, try again one at a time (in case it is one value hanging us up)
	if err != nil {
		for _, s := range batch {
			_, err = b.writeStatusUpdatesToDB(ctx, []*models.StatusUpdate{s})
			if err != nil {
				log := log.With("msg_uuid", s.MsgUUID())

				if qerr := dbutil.AsQueryError(err); qerr != nil {
					query, params := qerr.Query()
					log = log.With("sql", query, "sql_params", params)
				}

				log.Error("error writing msg status", "error", err)

				err := courier.WriteToSpool(spoolDir, "statuses", s)
				if err != nil {
					log.Error("error writing status to spool", "error", err) // just have to log and move on
				}
			}
		}
	} else {
		for _, s := range unresolved {
			log.Warn(fmt.Sprintf("unable to find message with channel_id=%d and external_id=%s", s.ChannelID_, s.ExternalID_))
		}
	}
}

// writes a batch of msg status updates to the database - messages that can't be resolved are returned and aren't
// considered an error
func (b *backend) writeStatusUpdatesToDB(ctx context.Context, statuses []*models.StatusUpdate) ([]*models.StatusUpdate, error) {
	// get the statuses which are missing msg UUIDs
	missingUUID := make([]*models.StatusUpdate, 0, len(statuses))
	for _, s := range statuses {
		if s.MsgUUID_ == "" {
			missingUUID = append(missingUUID, s)
		}
	}

	if len(missingUUID) > 0 {
		if err := b.resolveStatusUpdateByExternalID(ctx, missingUUID); err != nil {
			return nil, fmt.Errorf("error resolving status updates by external ID: %w", err)
		}
	}

	resolved := make([]*models.StatusUpdate, 0, len(statuses))
	unresolved := make([]*models.StatusUpdate, 0, len(statuses))

	for _, s := range statuses {
		if s.MsgUUID_ != "" {
			resolved = append(resolved, s)
		} else {
			unresolved = append(unresolved, s)
		}
	}

	if len(resolved) > 0 {
		_, err := models.WriteStatusUpdates(ctx, b.rt, resolved)
		if err != nil {
			return nil, fmt.Errorf("error writing resolved status updates: %w", err)
		}
	}

	return unresolved, nil
}

const sqlResolveStatusByExternalID = `
SELECT uuid, channel_id, external_id 
  FROM msgs_msg 
 WHERE (channel_id, external_id) IN (VALUES(:channel_id::int, :external_id))`

// tries to resolve msg UUIDs for the given statuses using their external IDs
func (b *backend) resolveStatusUpdateByExternalID(ctx context.Context, statuses []*models.StatusUpdate) error {
	rc := b.rt.VK.Get()
	defer rc.Close()

	chAndExtKeys := make([]string, len(statuses))
	for i, s := range statuses {
		chAndExtKeys[i] = fmt.Sprintf("%d|%s", s.ChannelID_, s.ExternalID_)
	}
	cachedUUIDs, err := b.sentExternalIDs.MGet(ctx, rc, chAndExtKeys...)
	if err != nil {
		// log error but we continue and try to get ids from the database
		slog.Error("error looking up sent message ids in valkey", "error", err)
	}

	// collect the statuses that couldn't be resolved from cache, update the ones that could
	notInCache := make([]*models.StatusUpdate, 0, len(statuses))
	for i, val := range cachedUUIDs {
		if val != "" && uuids.Is(val) {
			statuses[i].MsgUUID_ = models.MsgUUID(val)
		} else {
			notInCache = append(notInCache, statuses[i])
		}
	}

	if len(notInCache) == 0 {
		return nil
	}

	// create a mapping of channel id + external id -> status
	type ext struct {
		channelID  models.ChannelID
		externalID string
	}
	statusesByExt := make(map[ext]*models.StatusUpdate, len(notInCache))
	for _, s := range statuses {
		statusesByExt[ext{s.ChannelID_, s.ExternalID_}] = s
	}

	sql, params, err := dbutil.BulkSQL(b.rt.DB, sqlResolveStatusByExternalID, notInCache)
	if err != nil {
		return err
	}

	rows, err := b.rt.DB.QueryContext(ctx, sql, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var msgUUID models.MsgUUID
	var channelID models.ChannelID
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
