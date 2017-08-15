package rapidpro

import (
	"database/sql"

	null "gopkg.in/guregu/null.v3"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
)

// ContactURNID represents a contact urn's id
type ContactURNID struct {
	null.Int
}

// NilContactURNID is our nil value for ContactURNID
var NilContactURNID = ContactURNID{null.NewInt(0, false)}

// NewDBContactURN returns a new ContactURN object for the passed in org, contact and string urn, this is not saved to the DB yet
func newDBContactURN(org OrgID, channelID courier.ChannelID, contactID ContactID, urn courier.URN) *DBContactURN {
	return &DBContactURN{
		OrgID:     org,
		ChannelID: channelID,
		ContactID: contactID,
		Identity:  urn.Identity(),
		Scheme:    urn.Scheme(),
		Path:      urn.Path(),
		Display:   urn.Display(),
	}
}

const selectOrgURN = `
SELECT org_id, id, identity, scheme, path, display, priority, channel_id, contact_id 
FROM contacts_contacturn
WHERE org_id = $1 AND identity = $2
ORDER BY priority desc LIMIT 1
`

// contactURNForURN returns the ContactURN for the passed in org and URN, creating and associating
// it with the passed in contact if necessary
func contactURNForURN(db *sqlx.DB, org OrgID, channelID courier.ChannelID, contactID ContactID, urn courier.URN) (*DBContactURN, error) {
	contactURN := newDBContactURN(org, channelID, contactID, urn)
	err := db.Get(contactURN, selectOrgURN, org, urn)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// we didn't find it, let's insert it
	if err == sql.ErrNoRows {
		err = insertContactURN(db, contactURN)
		if err != nil {
			return nil, err
		}
	}

	// make sure our contact URN is up to date
	if contactURN.ChannelID != channelID || contactURN.ContactID != contactID {
		contactURN.ChannelID = channelID
		contactURN.ContactID = contactID
		err = updateContactURN(db, contactURN)
	}

	return contactURN, err
}

const insertURN = `
INSERT INTO contacts_contacturn(org_id, identity, path, scheme, display, priority, channel_id, contact_id)
VALUES(:org_id, :identity, :path, :scheme, :display, :priority, :channel_id, :contact_id)
RETURNING id
`

// InsertContactURN inserts the passed in urn, the id field will be populated with the result on success
func insertContactURN(db *sqlx.DB, urn *DBContactURN) error {
	rows, err := db.NamedQuery(insertURN, urn)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&urn.ID)
	}
	return err
}

const updateURN = `
UPDATE contacts_contacturn
SET channel_id = :channel_id, contact_id = :contact_id
WHERE id = :id
`

// UpdateContactURN updates the Channel and Contact on an existing URN
func updateContactURN(db *sqlx.DB, urn *DBContactURN) error {
	rows, err := db.NamedQuery(updateURN, urn)
	if err != nil {
		return err
	}
	if rows.Next() {
		rows.Scan(&urn.ID)
	}
	return err
}

// DBContactURN is our struct to map to database level URNs
type DBContactURN struct {
	OrgID     OrgID             `db:"org_id"`
	ID        ContactURNID      `db:"id"`
	Identity  string            `db:"identity"`
	Scheme    string            `db:"scheme"`
	Path      string            `db:"path"`
	Display   null.String       `db:"display"`
	Priority  int               `db:"priority"`
	ChannelID courier.ChannelID `db:"channel_id"`
	ContactID ContactID         `db:"contact_id"`
}
