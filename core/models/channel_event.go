package models

import (
	"context"
	"time"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
	"github.com/vinovest/sqlx"
)

// ChannelEvent is our typing of a channelevent's UUID
type ChannelEventUUID uuids.UUID

// ChannelEventType is the type of channel event this is
type ChannelEventType string

// Possible values for ChannelEventTypes
const (
	EventTypeNewConversation ChannelEventType = "new_conversation"
	EventTypeReferral        ChannelEventType = "referral"
	EventTypeStopContact     ChannelEventType = "stop_contact"
	EventTypeWelcomeMessage  ChannelEventType = "welcome_message"
)

// ChannelEvent represents an event on a channel.. that isn't a new message or status update
type ChannelEvent struct {
	UUID_         ChannelEventUUID `db:"uuid"               json:"uuid"`
	OrgID_        OrgID            `db:"org_id"             json:"org_id"`
	ChannelID_    ChannelID        `db:"channel_id"         json:"channel_id"`
	URN_          urns.URN         `db:"urn"                json:"urn"`
	EventType_    ChannelEventType `db:"event_type"         json:"event_type"`
	Extra_        null.Map[string] `db:"extra"              json:"extra"`
	OccurredOn_   time.Time        `db:"occurred_on"        json:"occurred_on"`
	ContactID_    ContactID        `db:"contact_id"         json:"-"`
	ContactURNID_ ContactURNID     `db:"contact_urn_id"     json:"-"`
	LogUUIDs      pq.StringArray   `db:"log_uuids"          json:"log_uuids"`

	// not stored on the event itself but included in spool files and needed for queueing to mailroom
	ChannelUUID_ ChannelUUID `db:"-" json:"channel_uuid"`
	ContactName_ string      `db:"-" json:"contact_name"`

	Channel_ *Channel `db:"-" json:"-"`
}

// NewChannelEvent creates a new channel event for the given channel and event type
func NewChannelEvent(channel *Channel, eventType ChannelEventType, urn urns.URN, clogUUID clogs.UUID) *ChannelEvent {
	return &ChannelEvent{
		UUID_:       ChannelEventUUID(uuids.NewV7()),
		OrgID_:      channel.OrgID(),
		ChannelID_:  channel.ID(),
		URN_:        urn,
		EventType_:  eventType,
		OccurredOn_: time.Now().In(time.UTC),
		LogUUIDs:    pq.StringArray{string(clogUUID)},

		ChannelUUID_: channel.UUID(),
		Channel_:     channel,
	}
}

func (e *ChannelEvent) EventUUID() uuids.UUID       { return uuids.UUID(e.UUID_) }
func (e *ChannelEvent) UUID() ChannelEventUUID      { return e.UUID_ }
func (e *ChannelEvent) OrgID() OrgID                { return e.OrgID_ }
func (e *ChannelEvent) ChannelID() ChannelID        { return e.ChannelID_ }
func (e *ChannelEvent) ChannelUUID() ChannelUUID    { return e.ChannelUUID_ }
func (e *ChannelEvent) Channel() *Channel           { return e.Channel_ }
func (e *ChannelEvent) URN() urns.URN               { return e.URN_ }
func (e *ChannelEvent) EventType() ChannelEventType { return e.EventType_ }
func (e *ChannelEvent) Extra() map[string]string    { return e.Extra_ }
func (e *ChannelEvent) OccurredOn() time.Time       { return e.OccurredOn_ }

func (e *ChannelEvent) WithContactName(name string) *ChannelEvent { e.ContactName_ = name; return e }
func (e *ChannelEvent) WithExtra(extra map[string]string) *ChannelEvent {
	e.Extra_ = null.Map[string](extra)
	return e
}
func (e *ChannelEvent) WithOccurredOn(t time.Time) *ChannelEvent { e.OccurredOn_ = t; return e }

const sqlInsertChannelEvent = `
INSERT INTO
	channels_channelevent( org_id,  uuid,  channel_id,  contact_id,  contact_urn_id,  event_type,  extra,  occurred_on,  created_on, status,  log_uuids)
				   VALUES(:org_id, :uuid, :channel_id, :contact_id, :contact_urn_id, :event_type, :extra, :occurred_on,       NOW(),    'P', :log_uuids)`

// InsertChannelEvent inserts the passed in channel event into the database
func InsertChannelEvent(ctx context.Context, db *sqlx.DB, e *ChannelEvent) error {
	_, err := db.NamedExecContext(ctx, sqlInsertChannelEvent, e)
	return err
}
