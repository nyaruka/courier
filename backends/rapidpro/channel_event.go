package rapidpro

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	null "gopkg.in/guregu/null.v3"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
)

// ChannelEventID is the type of our channel event ids
type ChannelEventID struct {
	null.Int
}

// String satisfies the Stringer interface
func (i ChannelEventID) String() string {
	if i.Valid {
		return strconv.FormatInt(i.Int64, 10)
	}
	return "null"
}

// newChannelEvent creates a new channel event
func newChannelEvent(channel courier.Channel, eventType courier.ChannelEventType, urn urns.URN) *DBChannelEvent {
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
	}
}

// writeChannelEvent writes the passed in event to the database, queueing it to our spool in case the database is down
func writeChannelEvent(b *backend, event courier.ChannelEvent) error {
	dbEvent := event.(*DBChannelEvent)

	err := writeChannelEventToDB(b, dbEvent)

	// failed writing, write to our spool instead
	if err != nil {
		logrus.WithError(err).WithField("channel_id", dbEvent.ChannelID_.Int64).WithField("event_type", dbEvent.EventType_).Error("error writing channel event to db")
		err = courier.WriteToSpool(b.config.SpoolDir, "events", dbEvent)
	}

	return err
}

const insertChannelEventSQL = `
INSERT INTO channels_channelevent("org_id", "channel_id", "contact_id", "contact_urn_id", "event_type", "extra", "occurred_on", "created_on")
						   VALUES(:org_id, :channel_id, :contact_id, :contact_urn_id, :event_type, :extra, :occurred_on, :created_on)
RETURNING id
`

// writeChannelEventToDB writes the passed in msg status to our db
func writeChannelEventToDB(b *backend, e *DBChannelEvent) error {
	// grab the contact for this event
	contact, err := contactForURN(b.db, e.OrgID_, e.ChannelID_, e.URN_, e.ContactName_)
	if err != nil {
		return err
	}

	e.ContactID_ = contact.ID
	e.ContactURNID_ = contact.URNID

	rows, err := b.db.NamedQuery(insertChannelEventSQL, e)
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
	err = queueChannelEvent(rc, e.OrgID_, e.ContactID_, e.ID_)
	if err != nil {
		logrus.WithError(err).WithField("evt_id", e.ID_.Int64).Error("error queueing channel event")
	}

	return nil
}

func (b *backend) flushChannelEventFile(filename string, contents []byte) error {
	event := &DBChannelEvent{}
	err := json.Unmarshal(contents, event)
	if err != nil {
		log.Printf("ERROR unmarshalling spool file '%s', renaming: %s\n", filename, err)
		os.Rename(filename, fmt.Sprintf("%s.error", filename))
		return nil
	}

	// try to flush to our database
	return writeChannelEventToDB(b, event)
}

const selectEventSQL = `
SELECT org_id, channel_id, contact_id, contact_urn_id, event_type, extra, occurred_on, created_on
FROM channels_channelevent
WHERE id = $1
`

func readChannelEventFromDB(b *backend, id ChannelEventID) (*DBChannelEvent, error) {
	e := &DBChannelEvent{
		ID_: id,
	}
	err := b.db.Get(e, selectEventSQL, id)
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
	Extra_       *utils.NullMap           `json:"extra"                   db:"extra"`
	OccurredOn_  time.Time                `json:"occurred_on"             db:"occurred_on"`
	CreatedOn_   time.Time                `json:"created_on"              db:"created_on"`

	ContactName_  string       `json:"contact_name"`
	ContactID_    ContactID    `json:"-"               db:"contact_id"`
	ContactURNID_ ContactURNID `json:"-"               db:"contact_urn_id"`

	logs []*courier.ChannelLog
}

func (e *DBChannelEvent) EventID() int64                      { return e.ID_.Int64 }
func (e *DBChannelEvent) ChannelID() courier.ChannelID        { return e.ChannelID_ }
func (e *DBChannelEvent) ChannelUUID() courier.ChannelUUID    { return e.ChannelUUID_ }
func (e *DBChannelEvent) ContactName() string                 { return e.ContactName_ }
func (e *DBChannelEvent) URN() urns.URN                       { return e.URN_ }
func (e *DBChannelEvent) EventType() courier.ChannelEventType { return e.EventType_ }
func (e *DBChannelEvent) OccurredOn() time.Time               { return e.OccurredOn_ }
func (e *DBChannelEvent) CreatedOn() time.Time                { return e.CreatedOn_ }

func (e *DBChannelEvent) WithContactName(name string) courier.ChannelEvent {
	e.ContactName_ = name
	return e
}
func (e *DBChannelEvent) WithExtra(extra map[string]interface{}) courier.ChannelEvent {
	newExtra := utils.NewNullMap(extra)
	e.Extra_ = &newExtra
	return e
}

func (e *DBChannelEvent) WithOccurredOn(time time.Time) courier.ChannelEvent {
	e.OccurredOn_ = time
	return e
}

func (e *DBChannelEvent) Logs() []*courier.ChannelLog    { return e.logs }
func (e *DBChannelEvent) AddLog(log *courier.ChannelLog) { e.logs = append(e.logs, log) }
