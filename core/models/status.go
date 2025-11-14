package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
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
    FROM 
        (VALUES(:msg_uuid::uuid, :channel_id::int, :status, :external_id, :log_uuid::uuid)) AS s(msg_uuid, channel_id, status, external_id, log_uuid),
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
		"status":     dynamoStatuses[s.MsgStatus],
	}
	if s.MsgStatus == MsgStatusFailed && s.FailedReason == "E" {
		data["reason"] = "error_limit"
	}

	return &dynamo.Item{Key: s.DynamoKey(), OrgID: int(s.OrgID), Data: data}, nil
}

var dynamoStatuses = map[MsgStatus]string{
	MsgStatusWired:     "wired",
	MsgStatusSent:      "sent",
	MsgStatusDelivered: "delivered",
	MsgStatusRead:      "read",
	MsgStatusErrored:   "errored",
	MsgStatusFailed:    "failed",
}
