package rapidpro

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null/v2"
	"github.com/sirupsen/logrus"
)

// ChannelEventID is the type of our channel event ids
type ChannelEventID null.Int

const NilChannelEventID = ChannelEventID(0)

func (i *ChannelEventID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ChannelEventID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ChannelEventID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ChannelEventID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// String satisfies the Stringer interface
func (i ChannelEventID) String() string {
	if i != NilChannelEventID {
		return strconv.FormatInt(int64(i), 10)
	}
	return "null"
}

// newChannelEvent creates a new channel event
func newChannelEvent(channel courier.Channel, eventType courier.ChannelEventType, urn urns.URN, clog *courier.ChannelLog) *DBChannelEvent {
	dbChannel := channel.(*DBChannel)
	now := time.Now().In(time.UTC)

	return &DBChannelEvent{
		ChannelUUID_: dbChannel.UUID_,
		OrgID_:       dbChannel.OrgID_,
		ChannelID_:   dbChannel.ID_,
		URN_:         urn,
		EventType_:   eventType,
		OccurredOn_:  now,
		CreatedOn_:   now,
		LogUUIDs:     []string{string(clog.UUID())},

		channel: dbChannel,
	}
}

// writeChannelEvent writes the passed in event to the database, queueing it to our spool in case the database is down
func writeChannelEvent(ctx context.Context, b *backend, event courier.ChannelEvent, clog *courier.ChannelLog) error {
	dbEvent := event.(*DBChannelEvent)

	err := writeChannelEventToDB(ctx, b, dbEvent, clog)

	// failed writing, write to our spool instead
	if err != nil {
		logrus.WithError(err).WithField("channel_id", dbEvent.ChannelID).WithField("event_type", dbEvent.EventType_).Error("error writing channel event to db")
	}

	if err != nil {
		err = courier.WriteToSpool(b.config.SpoolDir, "events", dbEvent)
	}

	return err
}

const sqlInsertChannelEvent = `
INSERT INTO 
	channels_channelevent( org_id,  channel_id,  contact_id,  contact_urn_id,  event_type,  extra,  occurred_on,  created_on,  log_uuids)
				   VALUES(:org_id, :channel_id, :contact_id, :contact_urn_id, :event_type, :extra, :occurred_on, :created_on, :log_uuids)
RETURNING id`

// writeChannelEventToDB writes the passed in msg status to our db
func writeChannelEventToDB(ctx context.Context, b *backend, e *DBChannelEvent, clog *courier.ChannelLog) error {
	// grab the contact for this event
	contact, err := contactForURN(ctx, b, e.OrgID_, e.channel, e.URN_, "", e.ContactName_, clog)
	if err != nil {
		return err
	}

	// set our contact and urn id
	e.ContactID_ = contact.ID_
	e.ContactURNID_ = contact.URNID_

	rows, err := b.db.NamedQueryContext(ctx, sqlInsertChannelEvent, e)
	if err != nil {
		return err
	}
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&e.ID_)
	if err != nil {
		return err
	}

	// queue it up for handling by RapidPro
	rc := b.redisPool.Get()
	defer rc.Close()

	// if we had a problem queueing the event, log it
	err = queueChannelEvent(rc, contact, e)
	if err != nil {
		logrus.WithError(err).WithField("evt_id", e.ID_).Error("error queueing channel event")
	}

	return nil
}

func (b *backend) flushChannelEventFile(filename string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	event := &DBChannelEvent{}
	err := json.Unmarshal(contents, event)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// look up our channel
	channel, err := b.GetChannel(ctx, courier.AnyChannelType, event.ChannelUUID_)
	if err != nil {
		return err
	}
	event.channel = channel.(*DBChannel)

	// create log tho it won't be written
	clog := courier.NewChannelLog(courier.ChannelLogTypeMsgReceive, channel, nil)

	// try to flush to our database
	return writeChannelEventToDB(ctx, b, event, clog)
}

const sqlSelectEvent = `
SELECT org_id, channel_id, contact_id, contact_urn_id, event_type, extra, occurred_on, created_on, log_uuids
  FROM channels_channelevent
 WHERE id = $1`

func readChannelEventFromDB(b *backend, id ChannelEventID) (*DBChannelEvent, error) {
	e := &DBChannelEvent{
		ID_: id,
	}
	err := b.db.Get(e, sqlSelectEvent, id)
	return e, err
}

//-----------------------------------------------------------------------------
// ChannelEvent implementation
//-----------------------------------------------------------------------------

// DBChannelEvent represents an event on a channel
type DBChannelEvent struct {
	ID_          ChannelEventID           `                               db:"id"`
	OrgID_       OrgID                    `json:"org_id"                  db:"org_id"`
	ChannelUUID_ courier.ChannelUUID      `json:"channel_uuid"            db:"channel_uuid"`
	ChannelID_   courier.ChannelID        `json:"channel_id"              db:"channel_id"`
	URN_         urns.URN                 `json:"urn"                     db:"urn"`
	EventType_   courier.ChannelEventType `json:"event_type"              db:"event_type"`
	Extra_       null.Map                 `json:"extra"                   db:"extra"`
	OccurredOn_  time.Time                `json:"occurred_on"             db:"occurred_on"`
	CreatedOn_   time.Time                `json:"created_on"              db:"created_on"`
	LogUUIDs     pq.StringArray           `json:"log_uuids"               db:"log_uuids"`

	ContactName_  string       `json:"contact_name"`
	ContactID_    ContactID    `json:"-"               db:"contact_id"`
	ContactURNID_ ContactURNID `json:"-"               db:"contact_urn_id"`

	channel *DBChannel
}

func (e *DBChannelEvent) EventID() int64                      { return int64(e.ID_) }
func (e *DBChannelEvent) ChannelID() courier.ChannelID        { return e.ChannelID_ }
func (e *DBChannelEvent) ChannelUUID() courier.ChannelUUID    { return e.ChannelUUID_ }
func (e *DBChannelEvent) ContactName() string                 { return e.ContactName_ }
func (e *DBChannelEvent) URN() urns.URN                       { return e.URN_ }
func (e *DBChannelEvent) Extra() map[string]interface{}       { return e.Extra_ }
func (e *DBChannelEvent) EventType() courier.ChannelEventType { return e.EventType_ }
func (e *DBChannelEvent) OccurredOn() time.Time               { return e.OccurredOn_ }
func (e *DBChannelEvent) CreatedOn() time.Time                { return e.CreatedOn_ }
func (e *DBChannelEvent) Channel() *DBChannel                 { return e.channel }

func (e *DBChannelEvent) WithContactName(name string) courier.ChannelEvent {
	e.ContactName_ = name
	return e
}
func (e *DBChannelEvent) WithExtra(extra map[string]interface{}) courier.ChannelEvent {
	e.Extra_ = null.Map(extra)
	return e
}

func (e *DBChannelEvent) WithOccurredOn(time time.Time) courier.ChannelEvent {
	e.OccurredOn_ = time
	return e
}
