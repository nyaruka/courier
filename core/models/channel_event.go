package models

import (
	"context"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/svclogs"
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
	LogUUIDs     []svclogs.UUID    `json:"log_uuids"`

	// optional extra set by handlers, used to update the contact
	ContactName_ string `json:"contact_name,omitempty"`

	Channel_ *Channel `json:"-"`
}

// NewChannelEvent creates a new channel event for the given channel and event type
func NewChannelEvent(channel *Channel, eventType ChannelEventType, urn urns.URN, clog *ChannelLog) *ChannelEvent {
	return &ChannelEvent{
		UUID_:        ChannelEventUUID(uuids.NewV7()),
		ChannelUUID_: channel.UUID(),
		URN_:         urn,
		EventType_:   eventType,
		OccurredOn_:  time.Now().In(time.UTC),
		LogUUIDs:     []svclogs.UUID{clog.UUID},

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

// dbChannelEvent is the database representation of a channel event
type dbChannelEvent struct {
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

// WriteChannelEvent writes the passed in event to the database, or spools it if the database is unavailable
func WriteChannelEvent(ctx context.Context, rt *runtime.Runtime, event *ChannelEvent, clog *ChannelLog) error {
	timeout, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	err := writeChannelEventToDB(timeout, rt, event, clog)

	// failed writing, write to our spool instead
	if err != nil {
		slog.Error("error writing channel event to db", "error", err, "channel", event.ChannelUUID_, "event_type", event.EventType_)

		err = eventSpool.Add([]*ChannelEvent{event})
	}

	return err
}

// writeChannelEventToDB writes the passed in channel event to our db
func writeChannelEventToDB(ctx context.Context, rt *runtime.Runtime, e *ChannelEvent, clog *ChannelLog) error {
	// grab the contact for this event
	contact, err := contactForURN(ctx, rt, e.Channel_.OrgID(), e.Channel_, e.URN_, nil, e.ContactName_, true, clog)
	if err != nil {
		return err
	}

	if err := InsertChannelEvent(ctx, rt.DB, e, contact); err != nil {
		return err
	}

	// queue it up for handling by mailroom
	rc := rt.VK.Get()
	defer rc.Close()

	// if we had a problem queueing the event, log it
	if err := queueEventHandling(ctx, rc, contact, e); err != nil {
		slog.Error("error queueing channel event", "error", err, "event", e.UUID_)
	}

	return nil
}

// flushEvents is the flush function for the event spool - it retries writing spooled channel events to the database,
// returning those that fail again so they're respooled
func flushEvents(ctx context.Context, rt *runtime.Runtime, batch []*ChannelEvent) ([]*ChannelEvent, error) {
	var failed []*ChannelEvent

	for _, event := range batch {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		err := flushEvent(ctx, rt, event)
		cancel()

		if err != nil {
			slog.Error("error flushing spooled channel event", "error", err, "event", event.UUID_)
			failed = append(failed, event)
		}
	}

	return failed, nil
}

func flushEvent(ctx context.Context, rt *runtime.Runtime, event *ChannelEvent) error {
	// look up our channel
	channel, err := GetChannel(ctx, AnyChannelType, event.ChannelUUID_)
	if err != nil {
		return err
	}
	event.Channel_ = channel

	// create log tho it won't be written
	clog := NewChannelLog(ChannelLogTypeMsgReceive, channel, nil, nil)

	// try to flush to our database
	return writeChannelEventToDB(ctx, rt, event, clog)
}

// InsertChannelEvent inserts the passed in channel event into the database
func InsertChannelEvent(ctx context.Context, db *sqlx.DB, e *ChannelEvent, contact *Contact) error {
	logUUIDs := make(pq.StringArray, len(e.LogUUIDs))
	for i := range e.LogUUIDs {
		logUUIDs[i] = string(e.LogUUIDs[i])
	}

	row := &dbChannelEvent{
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
