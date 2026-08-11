package rapidpro

import (
	"context"
	"log/slog"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
)

// writeChannelEvent writes the passed in event to the database, queueing it to our spool in case the database is down
func writeChannelEvent(ctx context.Context, b *backend, event *models.ChannelEvent, clog *models.ChannelLog) error {
	err := writeChannelEventToDB(ctx, b, event, clog)

	// failed writing, write to our spool instead
	if err != nil {
		slog.Error("error writing channel event to db", "error", err, "channel", event.ChannelUUID_, "event_type", event.EventType_)
	}

	if err != nil {
		err = b.eventSpool.Add([]*models.ChannelEvent{event})
	}

	return err
}

// writeChannelEventToDB writes the passed in channel event to our db
func writeChannelEventToDB(ctx context.Context, b *backend, e *models.ChannelEvent, clog *models.ChannelLog) error {
	// grab the contact for this event
	contact, err := contactForURN(ctx, b, e.Channel_.OrgID(), e.Channel_, e.URN_, nil, e.ContactName_, true, clog)
	if err != nil {
		return err
	}

	if err := models.InsertChannelEvent(ctx, b.rt.DB, e, contact); err != nil {
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
func (b *backend) flushEvents(ctx context.Context, batch []*models.ChannelEvent) ([]*models.ChannelEvent, error) {
	var failed []*models.ChannelEvent

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

func (b *backend) flushEvent(ctx context.Context, event *models.ChannelEvent) error {
	// look up our channel
	channel, err := b.GetChannel(ctx, models.AnyChannelType, event.ChannelUUID_)
	if err != nil {
		return err
	}
	event.Channel_ = channel

	// create log tho it won't be written
	clog := models.NewChannelLog(models.ChannelLogTypeMsgReceive, channel, nil, nil)

	// try to flush to our database
	return writeChannelEventToDB(ctx, b, event, clog)
}
