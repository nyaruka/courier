package rapidpro

import (
	"time"

	null "gopkg.in/guregu/null.v3"

	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// ContactID is our representation of our database contact id
type ContactID struct {
	null.Int
}

// NilContactID represents our nil value for ContactID
var NilContactID = ContactID{null.NewInt(0, false)}

const insertContactSQL = `
INSERT INTO contacts_contact(org_id, is_active, is_blocked, is_test, is_stopped, uuid, created_on, modified_on, created_by_id, modified_by_id, name) 
VALUES(:org_id, TRUE, FALSE, FALSE, FALSE, :uuid, :created_on, :modified_on, :created_by_id, :modified_by_id, :name)
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
		err = rows.Scan(&contact.ID)
	}
	return err
}

const lookupContactFromURNSQL = `
SELECT c.org_id, c.id, c.uuid, c.modified_on, c.created_on, c.name, u.id as "urn_id"
FROM contacts_contact AS c, contacts_contacturn AS u 
WHERE u.identity = $1 AND u.contact_id = c.id AND u.org_id = $2 AND c.is_active = TRUE AND c.is_test = FALSE
`

// contactForURN first tries to look up a contact for the passed in URN, if not finding one then creating one
func contactForURN(db *sqlx.DB, org OrgID, channelID courier.ChannelID, urn urns.URN, name string) (*DBContact, error) {
	// try to look up our contact by URN
	contact := &DBContact{}
	err := db.Get(contact, lookupContactFromURNSQL, urn.Identity(), org)
	if err != nil && err != sql.ErrNoRows {
		logrus.WithError(err).WithField("urn", urn.Identity()).WithField("org_id", org).Error("error looking up contact")
		return nil, err
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		err := setDefaultURN(db, channelID, contact, urn)
		return contact, err
	}

	// didn't find it, we need to create it instead
	contact.OrgID = org
	contact.UUID = uuid.NewV4().String()
	contact.CreatedOn = time.Now()
	contact.ModifiedOn = time.Now()
	contact.IsNew = true

	// TODO: don't set name for anonymous orgs
	if name != "" {
		contact.Name = null.StringFrom(name)
	}

	// TODO: Set these to a system user
	contact.CreatedBy = 1
	contact.ModifiedBy = 1

	// insert it
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}

	err = insertContact(tx, contact)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// associate our URN
	// If we've inserted a duplicate URN then we'll get a uniqueness violation.
	// That means this contact URN was written by someone else after we tried to look it up.
	contactURN, err := contactURNForURN(tx, org, channelID, contact.ID, urn)
	if err != nil {
		tx.Rollback()
		if pqErr, ok := err.(*pq.Error); ok {
			// if this was a duplicate URN, start over with a contact lookup
			if pqErr.Code.Name() == "unique_violation" {
				return contactForURN(db, org, channelID, urn, name)
			}
		}
		return nil, err
	}

	// if the returned URN is for a different contact, then we were in a race as well, rollback and start over
	if contactURN.ContactID.Int64 != contact.ID.Int64 {
		tx.Rollback()
		return contactForURN(db, org, channelID, urn, name)
	}

	// all is well, we created the new contact, commit and move forward
	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	// store this URN on our contact
	contact.URNID = contactURN.ID

	// and return it
	return contact, nil
}

// DBContact is our struct for a contact in the database
type DBContact struct {
	OrgID OrgID       `db:"org_id"`
	ID    ContactID   `db:"id"`
	UUID  string      `db:"uuid"`
	Name  null.String `db:"name"`

	URNID ContactURNID `db:"urn_id"`

	CreatedOn  time.Time `db:"created_on"`
	ModifiedOn time.Time `db:"modified_on"`

	CreatedBy  int `db:"created_by_id"`
	ModifiedBy int `db:"modified_by_id"`

	IsNew bool
}
