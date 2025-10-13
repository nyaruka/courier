package rapidpro

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// ChannelEvent represents an event on a channel.. that isn't a new message or status update
type ChannelEvent struct {
	ID_          models.ChannelEventID   `                               db:"id"`
	UUID_        models.ChannelEventUUID `json:"uuid"                    db:"uuid"`
	OrgID_       models.OrgID            `json:"org_id"                  db:"org_id"`
	ChannelUUID_ models.ChannelUUID      `json:"channel_uuid"            db:"channel_uuid"`
	ChannelID_   models.ChannelID        `json:"channel_id"              db:"channel_id"`
	URN_         urns.URN                `json:"urn"                     db:"urn"`
	EventType_   models.ChannelEventType `json:"event_type"              db:"event_type"`
	OptInID_     null.Int                `json:"optin_id"                db:"optin_id"`
	Extra_       null.Map[string]        `json:"extra"                   db:"extra"`
	OccurredOn_  time.Time               `json:"occurred_on"             db:"occurred_on"`
	CreatedOn_   time.Time               `json:"created_on"              db:"created_on"`
	LogUUIDs     dbutil.StringArray      `json:"log_uuids"               db:"log_uuids"`

	ContactID_    models.ContactID    `json:"-"               db:"contact_id"`
	ContactURNID_ models.ContactURNID `json:"-"               db:"contact_urn_id"`

	// used to update contact
	ContactName_   string            `json:"contact_name"`
	URNAuthTokens_ map[string]string `json:"auth_tokens"`

	channel *models.Channel
}

// newChannelEvent creates a new channel event
func newChannelEvent(channel courier.Channel, eventType models.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) *ChannelEvent {
	dbChannel := channel.(*models.Channel)

	return &ChannelEvent{
		UUID_:        models.ChannelEventUUID(uuids.NewV7()),
		ChannelUUID_: dbChannel.UUID_,
		OrgID_:       dbChannel.OrgID_,
		ChannelID_:   dbChannel.ID_,
		URN_:         urn,
		EventType_:   eventType,
		OccurredOn_:  time.Now().In(time.UTC),
		LogUUIDs:     dbutil.StringArray{string(clog.UUID)},

		channel: dbChannel,
	}
}

func (e *ChannelEvent) EventUUID() uuids.UUID              { return uuids.UUID(e.UUID_) }
func (e *ChannelEvent) UUID() models.ChannelEventUUID      { return e.UUID_ }
func (e *ChannelEvent) ChannelID() models.ChannelID        { return e.ChannelID_ }
func (e *ChannelEvent) ChannelUUID() models.ChannelUUID    { return e.ChannelUUID_ }
func (e *ChannelEvent) EventType() models.ChannelEventType { return e.EventType_ }
func (e *ChannelEvent) URN() urns.URN                      { return e.URN_ }
func (e *ChannelEvent) Extra() map[string]string           { return e.Extra_ }
func (e *ChannelEvent) OccurredOn() time.Time              { return e.OccurredOn_ }
func (e *ChannelEvent) CreatedOn() time.Time               { return e.CreatedOn_ }
func (e *ChannelEvent) Channel() *models.Channel           { return e.channel }

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
		slog.Error("error writing channel event to db", "error", err, "channel_id", dbEvent.ChannelID, "event_type", dbEvent.EventType_)
	}

	if err != nil {
		err = courier.WriteToSpool(b.rt.Config.SpoolDir, "events", dbEvent)
	}

	return err
}

const sqlInsertChannelEvent = `
INSERT INTO 
	channels_channelevent( org_id,  uuid, channel_id,  contact_id,  contact_urn_id,  event_type,  optin_id,  extra,  occurred_on, created_on, status,  log_uuids)
				   VALUES(:org_id, :uuid, :channel_id, :contact_id, :contact_urn_id, :event_type, :optin_id, :extra, :occurred_on,      NOW(), 'P',    :log_uuids)
RETURNING id, created_on`

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

	rows, err := b.rt.DB.NamedQueryContext(ctx, sqlInsertChannelEvent, e)
	if err != nil {
		return err
	}
	defer rows.Close()

	rows.Next()

	if err = rows.Scan(&e.ID_, &e.CreatedOn_); err != nil {
		return err
	}

	// queue it up for handling by RapidPro
	rc := b.rt.VK.Get()
	defer rc.Close()

	// if we had a problem queueing the event, log it
	err = queueEventHandling(ctx, rc, contact, e)
	if err != nil {
		slog.Error("error queueing channel event", "error", err, "evt_id", e.ID_)
	}

	return nil
}

func (b *backend) flushChannelEventFile(filename string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	event := &ChannelEvent{}
	err := json.Unmarshal(contents, event)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

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
