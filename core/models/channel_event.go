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

// ChannelEvent is an event on a channel - that isn't a new message or status update - as created by a handler, and
// is also what we marshal to spool files when the database is down. It doesn't carry any database ids - those are
// resolved when it's written.
type ChannelEvent struct {
	UUID_        ChannelEventUUID  `json:"uuid"`
	ChannelUUID_ ChannelUUID       `json:"channel_uuid"`
	URN_         urns.URN          `json:"urn"`
	EventType_   ChannelEventType  `json:"event_type"`
	Extra_       map[string]string `json:"extra,omitempty"`
	OccurredOn_  time.Time         `json:"occurred_on"`
	LogUUIDs     []clogs.UUID      `json:"log_uuids"`

	// optional extra set by handlers, used to update the contact
	ContactName_ string `json:"contact_name,omitempty"`

	Channel_ *Channel `json:"-"`
}

// NewChannelEvent creates a new channel event for the given channel and event type
func NewChannelEvent(channel *Channel, eventType ChannelEventType, urn urns.URN, clogUUID clogs.UUID) *ChannelEvent {
	return &ChannelEvent{
		UUID_:        ChannelEventUUID(uuids.NewV7()),
		ChannelUUID_: channel.UUID(),
		URN_:         urn,
		EventType_:   eventType,
		OccurredOn_:  time.Now().In(time.UTC),
		LogUUIDs:     []clogs.UUID{clogUUID},

		Channel_: channel,
	}
}

func (e *ChannelEvent) EventUUID() uuids.UUID       { return uuids.UUID(e.UUID_) }
func (e *ChannelEvent) UUID() ChannelEventUUID      { return e.UUID_ }
func (e *ChannelEvent) ChannelUUID() ChannelUUID    { return e.ChannelUUID_ }
func (e *ChannelEvent) Channel() *Channel           { return e.Channel_ }
func (e *ChannelEvent) URN() urns.URN               { return e.URN_ }
func (e *ChannelEvent) EventType() ChannelEventType { return e.EventType_ }
func (e *ChannelEvent) Extra() map[string]string    { return e.Extra_ }
func (e *ChannelEvent) OccurredOn() time.Time       { return e.OccurredOn_ }

func (e *ChannelEvent) WithContactName(name string) *ChannelEvent { e.ContactName_ = name; return e }
func (e *ChannelEvent) WithExtra(extra map[string]string) *ChannelEvent {
	e.Extra_ = extra
	return e
}
func (e *ChannelEvent) WithOccurredOn(t time.Time) *ChannelEvent { e.OccurredOn_ = t; return e }

// channelEventRow is the database representation of a channel event
type channelEventRow struct {
	UUID         ChannelEventUUID `db:"uuid"`
	OrgID        OrgID            `db:"org_id"`
	ChannelID    ChannelID        `db:"channel_id"`
	URN          urns.URN         `db:"urn"`
	EventType    ChannelEventType `db:"event_type"`
	Extra        null.Map[string] `db:"extra"`
	OccurredOn   time.Time        `db:"occurred_on"`
	ContactID    ContactID        `db:"contact_id"`
	ContactURNID ContactURNID     `db:"contact_urn_id"`
	LogUUIDs     pq.StringArray   `db:"log_uuids"`
}

const sqlInsertChannelEvent = `
INSERT INTO
	channels_channelevent( org_id,  uuid,  channel_id,  contact_id,  contact_urn_id,  event_type,  extra,  occurred_on,  created_on, status,  log_uuids)
				   VALUES(:org_id, :uuid, :channel_id, :contact_id, :contact_urn_id, :event_type, :extra, :occurred_on,       NOW(),    'P', :log_uuids)`

// InsertChannelEvent inserts the passed in channel event into the database
func InsertChannelEvent(ctx context.Context, db *sqlx.DB, e *ChannelEvent, contact *Contact) error {
	logUUIDs := make(pq.StringArray, len(e.LogUUIDs))
	for i := range e.LogUUIDs {
		logUUIDs[i] = string(e.LogUUIDs[i])
	}

	row := &channelEventRow{
		UUID:         e.UUID_,
		OrgID:        e.Channel_.OrgID(),
		ChannelID:    e.Channel_.ID(),
		URN:          e.URN_,
		EventType:    e.EventType_,
		Extra:        null.Map[string](e.Extra_),
		OccurredOn:   e.OccurredOn_,
		ContactID:    contact.ID_,
		ContactURNID: contact.URNID_,
		LogUUIDs:     logUUIDs,
	}

	_, err := db.NamedExecContext(ctx, sqlInsertChannelEvent, row)
	return err
}
