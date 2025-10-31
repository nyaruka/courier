package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
)

// StatusUpdate represents a status update on a message
type StatusUpdate struct {
	ChannelUUID_ ChannelUUID `json:"channel_uuid"             db:"channel_uuid"`
	ChannelID_   ChannelID   `json:"channel_id"               db:"channel_id"`
	MsgUUID_     MsgUUID     `json:"msg_uuid,omitempty"       db:"msg_uuid"`
	OldURN_      urns.URN    `json:"old_urn"                  db:"old_urn"`
	NewURN_      urns.URN    `json:"new_urn"                  db:"new_urn"`
	ExternalID_  string      `json:"external_id,omitempty"    db:"external_id"`
	Status_      MsgStatus   `json:"status"                   db:"status"`
	ModifiedOn_  time.Time   `json:"modified_on"              db:"modified_on"`
	LogUUID      clogs.UUID  `json:"log_uuid"                 db:"log_uuid"`
}

func (s *StatusUpdate) EventUUID() uuids.UUID    { return uuids.UUID(s.MsgUUID_) }
func (s *StatusUpdate) ChannelUUID() ChannelUUID { return s.ChannelUUID_ }
func (s *StatusUpdate) MsgUUID() MsgUUID         { return s.MsgUUID_ }

func (s *StatusUpdate) SetURNUpdate(old, new urns.URN) error {
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
func (s *StatusUpdate) URNUpdate() (urns.URN, urns.URN) {
	return s.OldURN_, s.NewURN_
}

func (s *StatusUpdate) ExternalID() string      { return s.ExternalID_ }
func (s *StatusUpdate) SetExternalID(id string) { s.ExternalID_ = id }

func (s *StatusUpdate) Status() MsgStatus          { return s.Status_ }
func (s *StatusUpdate) SetStatus(status MsgStatus) { s.Status_ = status }

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
	external_id = CASE WHEN s.external_id != '' THEN s.external_id ELSE msgs_msg.external_id END,
	modified_on = NOW(),
	log_uuids = array_append(log_uuids, s.log_uuid)
 FROM (VALUES(:msg_uuid::uuid, :channel_id::int, :status, :external_id, :log_uuid::uuid)) AS s(msg_uuid, channel_id, status, external_id, log_uuid) 
WHERE msgs_msg.uuid = s.msg_uuid AND msgs_msg.channel_id = s.channel_id AND msgs_msg.direction = 'O'
`

func WriteStatusUpdates(ctx context.Context, rt *runtime.Runtime, statuses []*StatusUpdate) error {
	if err := dbutil.BulkQuery(ctx, rt.DB, sqlUpdateMsgByUUID, statuses); err != nil {
		return fmt.Errorf("error writing statuses: %w", err)
	}
	return nil
}
