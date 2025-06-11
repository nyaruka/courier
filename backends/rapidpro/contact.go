package rapidpro

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
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

// Contact is our struct for a contact in the database
type Contact struct {
	OrgID_ OrgID               `db:"org_id"`
	ID_    ContactID           `db:"id"`
	UUID_  courier.ContactUUID `db:"uuid"`
	Name_  null.String         `db:"name"`

	URNID_ ContactURNID `db:"urn_id"`

	CreatedOn_  time.Time `db:"created_on"`
	ModifiedOn_ time.Time `db:"modified_on"`

	CreatedBy_  UserID `db:"created_by_id"`
	ModifiedBy_ UserID `db:"modified_by_id"`

	IsNew_ bool
}

// UUID returns the UUID for this contact
func (c *Contact) UUID() courier.ContactUUID { return c.UUID_ }

const sqlInsertContact = `
INSERT INTO 
	contacts_contact(org_id, is_active, status, uuid, created_on, modified_on, created_by_id, modified_by_id, name, ticket_count) 
              VALUES(:org_id, TRUE, 'A', :uuid, :created_on, :modified_on, :created_by_id, :modified_by_id, :name, 0)
RETURNING id
`

// insertContact inserts the passed in contact, the id field will be populated with the result on success
func insertContact(tx *sqlx.Tx, contact *Contact) error {
	rows, err := tx.NamedQuery(sqlInsertContact, contact)
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
func contactForURN(ctx context.Context, b *backend, org OrgID, channel *Channel, urn urns.URN, authTokens map[string]string, name string, allowCreate bool, clog *courier.ChannelLog) (*Contact, error) {
	log := slog.With("org_id", org, "urn", urn.Identity(), "channel_uuid", channel.UUID(), "log_uuid", clog.UUID)

	// try to look up our contact by URN
	contact := &Contact{}
	err := b.db.GetContext(ctx, contact, lookupContactFromURNSQL, urn.Identity(), org)
	if err != nil && err != sql.ErrNoRows {
		log.Error("error looking up contact by URN", "error", err)
		return nil, fmt.Errorf("error looking up contact by URN: %w", err)
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		tx, err := b.db.BeginTxx(ctx, nil)
		if err != nil {
			log.Error("error beginning transaction", "error", err)
			return nil, fmt.Errorf("error beginning transaction: %w", err)
		}

		// update contact's URNs so this URN has priority
		err = setDefaultURN(tx, channel, contact, urn, authTokens)
		if err != nil {
			log.Error("error updating default URN for contact", "error", err)
			tx.Rollback()
			return nil, fmt.Errorf("error setting default URN for contact: %w", err)
		}
		return contact, tx.Commit()
	}

	if !allowCreate {
		return nil, nil
	}

	// didn't find it, we need to create it instead
	contact.OrgID_ = org
	contact.UUID_ = courier.ContactUUID(uuids.NewV4())
	contact.CreatedOn_ = time.Now()
	contact.CreatedBy_ = b.systemUserID
	contact.ModifiedOn_ = time.Now()
	contact.ModifiedBy_ = b.systemUserID
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
						log.Error("unable to describe URN", "error", err)
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

	// insert it
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %w", err)
	}

	err = insertContact(tx, contact)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error inserting contact: %w", err)
	}

	// used for unit testing contact races
	if urnSleep {
		time.Sleep(time.Millisecond * 50)
	}

	// associate our URN
	// If we've inserted a duplicate URN then we'll get a uniqueness violation.
	// That means this contact URN was written by someone else after we tried to look it up.
	contactURN, err := getOrCreateContactURN(tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()

		if dbutil.IsUniqueViolation(err) {
			// if this was a duplicate URN, start over with a contact lookup
			return contactForURN(ctx, b, org, channel, urn, authTokens, name, true, clog)
		}
		return nil, fmt.Errorf("error getting URN for contact: %w", err)
	}

	// we stole the URN from another contact, roll back and start over
	if contactURN.PrevContactID != NilContactID {
		tx.Rollback()
		return contactForURN(ctx, b, org, channel, urn, authTokens, name, true, clog)
	}

	// all is well, we created the new contact, commit and move forward
	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("error commiting transaction: %w", err)
	}

	// store this URN on our contact
	contact.URNID_ = contactURN.ID

	b.stats.RecordContactCreated()

	return contact, nil
}
