package rapidpro

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/analytics"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// used by unit tests to slow down urn operations to test races
var urnSleep bool

// ContactID is our representation of our database contact id
type ContactID null.Int

// NilContactID represents our nil value for ContactID
var NilContactID = ContactID(0)

func (i *ContactID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ContactID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ContactID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ContactID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// String returns a string representation of the id
func (i ContactID) String() string {
	if i != NilContactID {
		return strconv.FormatInt(int64(i), 10)
	}
	return "null"
}

const insertContactSQL = `
INSERT INTO 
	contacts_contact(org_id, is_active, status, uuid, created_on, modified_on, created_by_id, modified_by_id, name, ticket_count) 
              VALUES(:org_id, TRUE, 'A', :uuid, :created_on, :modified_on, :created_by_id, :modified_by_id, :name, 0)
RETURNING id
`

// insertContact inserts the passed in contact, the id field will be populated with the result on success
func insertContact(tx *sqlx.Tx, contact *DBContact) error {
	rows, err := tx.NamedQuery(insertContactSQL, contact)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&contact.ID_)
	}
	return err
}

const lookupContactFromURNSQL = `
SELECT 
	c.org_id, 
	c.id, 
	c.uuid, 
	c.modified_on, 
	c.created_on, 
	c.name, 
	u.id as "urn_id"
FROM 
	contacts_contact AS c, 
	contacts_contacturn AS u 
WHERE 
	u.identity = $1 AND 
	u.contact_id = c.id AND 
	u.org_id = $2 AND 
	c.is_active = TRUE
`

// contactForURN first tries to look up a contact for the passed in URN, if not finding one then creating one
func contactForURN(ctx context.Context, b *backend, org OrgID, channel *DBChannel, urn urns.URN, auth string, name string, clog *courier.ChannelLog) (*DBContact, error) {
	// try to look up our contact by URN
	contact := &DBContact{}
	err := b.db.GetContext(ctx, contact, lookupContactFromURNSQL, urn.Identity(), org)
	if err != nil && err != sql.ErrNoRows {
		logrus.WithError(err).WithField("urn", urn.Identity()).WithField("org_id", org).Error("error looking up contact")
		return nil, errors.Wrap(err, "error looking up contact by URN")
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		// insert it
		tx, err := b.db.BeginTxx(ctx, nil)
		if err != nil {
			logrus.WithError(err).WithField("urn", urn.Identity()).WithField("org_id", org).Error("error looking up contact")
			return nil, errors.Wrap(err, "error beginning transaction")
		}

		err = setDefaultURN(tx, channel, contact, urn, auth)
		if err != nil {
			logrus.WithError(err).WithField("urn", urn.Identity()).WithField("org_id", org).Error("error looking up contact")
			tx.Rollback()
			return nil, errors.Wrap(err, "error setting default URN for contact")
		}
		return contact, tx.Commit()
	}

	// didn't find it, we need to create it instead
	contact.OrgID_ = org
	contact.UUID_ = courier.ContactUUID(uuids.New())
	contact.CreatedOn_ = time.Now()
	contact.ModifiedOn_ = time.Now()
	contact.IsNew_ = true

	// if we aren't an anonymous org, we want to look up a name if possible and set it
	if !channel.OrgIsAnon() {
		// no name was passed in, see if our handler can look up information for this URN
		if name == "" {
			handler := courier.GetHandler(channel.ChannelType())
			if handler != nil {
				describer, isDescriber := handler.(courier.URNDescriber)
				if isDescriber {
					attrs, err := describer.DescribeURN(ctx, channel, urn, clog)

					// in the case of errors, we log the error but move onwards anyways
					if err != nil {
						logrus.WithField("channel_uuid", channel.UUID()).WithField("channel_type", channel.ChannelType()).WithField("urn", urn).WithError(err).Error("unable to describe URN")
					} else {
						name = attrs["name"]
					}
				}
			}
		}

		if name != "" {
			if utf8.RuneCountInString(name) > 128 {
				name = string([]rune(name)[:127])
			}

			contact.Name_ = null.String(dbutil.ToValidUTF8(name))
		}
	}

	// TODO: Set these to a system user
	contact.CreatedBy_ = 1
	contact.ModifiedBy_ = 1

	// insert it
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error beginning transaction")
	}

	err = insertContact(tx, contact)
	if err != nil {
		tx.Rollback()
		return nil, errors.Wrap(err, "error inserting contact")
	}

	// used for unit testing contact races
	if urnSleep {
		time.Sleep(time.Millisecond * 50)
	}

	// associate our URN
	// If we've inserted a duplicate URN then we'll get a uniqueness violation.
	// That means this contact URN was written by someone else after we tried to look it up.
	contactURN, err := getOrCreateContactURN(tx, channel, contact.ID_, urn, auth)
	if err != nil {
		tx.Rollback()

		if dbutil.IsUniqueViolation(err) {
			// if this was a duplicate URN, start over with a contact lookup
			return contactForURN(ctx, b, org, channel, urn, auth, name, clog)
		}
		return nil, errors.Wrap(err, "error getting URN for contact")
	}

	// we stole the URN from another contact, roll back and start over
	if contactURN.PrevContactID != NilContactID {
		tx.Rollback()
		return contactForURN(ctx, b, org, channel, urn, auth, name, clog)
	}

	// all is well, we created the new contact, commit and move forward
	err = tx.Commit()
	if err != nil {
		return nil, errors.Wrap(err, "error commiting transaction")
	}

	// store this URN on our contact
	contact.URNID_ = contactURN.ID

	// log that we created a new contact to librato
	analytics.Gauge("courier.new_contact", float64(1))

	// and return it
	return contact, nil
}

// DBContact is our struct for a contact in the database
type DBContact struct {
	OrgID_ OrgID               `db:"org_id"`
	ID_    ContactID           `db:"id"`
	UUID_  courier.ContactUUID `db:"uuid"`
	Name_  null.String         `db:"name"`

	URNID_ ContactURNID `db:"urn_id"`

	CreatedOn_  time.Time `db:"created_on"`
	ModifiedOn_ time.Time `db:"modified_on"`

	CreatedBy_  int `db:"created_by_id"`
	ModifiedBy_ int `db:"modified_by_id"`

	IsNew_ bool
}

// UUID returns the UUID for this contact
func (c *DBContact) UUID() courier.ContactUUID { return c.UUID_ }
