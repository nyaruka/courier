package rapidpro

import (
	"time"

	null "gopkg.in/guregu/null.v3"

	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	uuid "github.com/satori/go.uuid"
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
func insertContact(db *sqlx.DB, contact *DBContact) error {
	rows, err := db.NamedQuery(insertContactSQL, contact)
	if err != nil {
		return err
	}
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
func contactForURN(db *sqlx.DB, org OrgID, channelID courier.ChannelID, urn courier.URN, name string) (*DBContact, error) {
	// try to look up our contact by URN
	contact := DBContact{}
	err := db.Get(&contact, lookupContactFromURNSQL, urn, org)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		return &contact, nil
	}

	// didn't find it, we need to create it instead
	contact.OrgID = org
	contact.UUID = uuid.NewV4().String()
	contact.Name = name
	contact.CreatedOn = time.Now()
	contact.ModifiedOn = time.Now()

	// TODO: Set these to a system user
	contact.CreatedBy = 1
	contact.ModifiedBy = 1

	// Insert it
	err = insertContact(db, &contact)
	if err != nil {
		return nil, err
	}

	// associate our URN
	contactURN, err := contactURNForURN(db, org, channelID, contact.ID, urn)
	if err != nil {
		return nil, err
	}

	// save this URN on our contact
	contact.URNID = contactURN.ID

	// and return it
	return &contact, err
}

// DBContact is our struct for a contact in the database
type DBContact struct {
	OrgID OrgID     `db:"org_id"`
	ID    ContactID `db:"id"`
	UUID  string    `db:"uuid"`
	Name  string    `db:"name"`

	URNID ContactURNID `db:"urn_id"`

	CreatedOn  time.Time `db:"created_on"`
	ModifiedOn time.Time `db:"modified_on"`

	CreatedBy  int `db:"created_by_id"`
	ModifiedBy int `db:"modified_by_id"`
}
