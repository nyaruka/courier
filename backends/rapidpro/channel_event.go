package rapidpro

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null/v3"
)

// ChannelEvent wraps a models.ChannelEvent with the transient fields needed for spooling and queueing to mailroom
type ChannelEvent struct {
	*models.ChannelEvent

	// needed for spool re-lookup of channel and for queueing to mailroom
	ChannelUUID_ models.ChannelUUID `json:"channel_uuid"`

	// used to update contact
	ContactName_   string            `json:"contact_name"`
	URNAuthTokens_ map[string]string `json:"auth_tokens"`

	channel *models.Channel
}

// newChannelEvent creates a new channel event
func newChannelEvent(channel courier.Channel, eventType models.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) *ChannelEvent {
	dbChannel := channel.(*models.Channel)

	return &ChannelEvent{
		ChannelEvent: models.NewChannelEvent(dbChannel, eventType, urn, clog.UUID),
		ChannelUUID_: dbChannel.UUID_,
		channel:      dbChannel,
	}
}

func (e *ChannelEvent) ChannelUUID() models.ChannelUUID { return e.ChannelUUID_ }
func (e *ChannelEvent) Channel() *models.Channel        { return e.channel }

func (e *ChannelEvent) WithContactName(name string) courier.ChannelEvent {
	e.ContactName_ = name
	return e
}

func (e *ChannelEvent) WithURNAuthTokens(tokens map[string]string) courier.ChannelEvent {
	e.URNAuthTokens_ = tokens
	return e
}

func (e *ChannelEvent) WithExtra(extra map[string]string) courier.ChannelEvent {
	if e.EventType_ == models.EventTypeOptIn || e.EventType_ == models.EventTypeOptOut {
		optInID := extra["payload"]
		if optInID != "" {
			asInt, _ := strconv.Atoi(optInID)
			e.OptInID_ = null.Int(asInt)
		}
	}

	e.Extra_ = null.Map[string](extra)
	return e
}

func (e *ChannelEvent) WithOccurredOn(time time.Time) courier.ChannelEvent {
	e.OccurredOn_ = time
	return e
}

// writeChannelEvent writes the passed in event to the database, queueing it to our spool in case the database is down
func writeChannelEvent(ctx context.Context, b *backend, event courier.ChannelEvent, clog *courier.ChannelLog) error {
	dbEvent := event.(*ChannelEvent)

	err := writeChannelEventToDB(ctx, b, dbEvent, clog)

	// failed writing, write to our spool instead
	if err != nil {
		slog.Error("error writing channel event to db", "error", err, "channel_id", dbEvent.ChannelID_, "event_type", dbEvent.EventType_)
	}

	if err != nil {
		err = b.eventSpool.Add([]*ChannelEvent{dbEvent})
	}

	return err
}

// writeChannelEventToDB writes the passed in channel event to our db
func writeChannelEventToDB(ctx context.Context, b *backend, e *ChannelEvent, clog *courier.ChannelLog) error {
	// grab the contact for this event
	contact, err := contactForURN(ctx, b, e.OrgID_, e.channel, e.URN_, e.URNAuthTokens_, e.ContactName_, true, clog)
	if err != nil {
		return err
	}

	// set our contact and urn id
	e.ContactID_ = contact.ID_
	e.ContactURNID_ = contact.URNID_

	if err := models.InsertChannelEvent(ctx, b.rt.DB, e.ChannelEvent); err != nil {
		return err
	}

	// queue it up for handling by RapidPro
	rc := b.rt.VK.Get()
	defer rc.Close()

	// if we had a problem queueing the event, log it
	if err := queueEventHandling(ctx, rc, contact, e); err != nil {
		slog.Error("error queueing channel event", "error", err, "event", e.UUID_)
	}

	return nil
}

// flushEvents is the flush function for the event spool - it retries writing spooled channel events to the database,
// returning those that fail again so they're respooled
func (b *backend) flushEvents(ctx context.Context, batch []*ChannelEvent) ([]*ChannelEvent, error) {
	var failed []*ChannelEvent

	for _, event := range batch {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		err := b.flushEvent(ctx, event)
		cancel()

		if err != nil {
			slog.Error("error flushing spooled channel event", "error", err, "event", event.UUID_)
			failed = append(failed, event)
		}
	}

	return failed, nil
}

func (b *backend) flushEvent(ctx context.Context, event *ChannelEvent) error {
	// look up our channel
	channel, err := b.GetChannel(ctx, models.AnyChannelType, event.ChannelUUID_)
	if err != nil {
		return err
	}
	event.channel = channel.(*models.Channel)

	// create log tho it won't be written
	clog := courier.NewChannelLog(courier.ChannelLogTypeMsgReceive, channel, nil)

	// try to flush to our database
	return writeChannelEventToDB(ctx, b, event, clog)
}
