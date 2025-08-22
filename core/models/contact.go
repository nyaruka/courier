package models

import (
	"context"
	"database/sql/driver"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

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

// ContactUUID is our typing of a contact's UUID
type ContactUUID uuids.UUID

// NilContactUUID is our nil value for contact UUIDs
var NilContactUUID = ContactUUID("")

// Contact is our struct for a contact in the database
type Contact struct {
	OrgID_ OrgID       `db:"org_id"`
	ID_    ContactID   `db:"id"`
	UUID_  ContactUUID `db:"uuid"`
	Name_  null.String `db:"name"`

	URNID_ ContactURNID `db:"urn_id"`

	CreatedOn_  time.Time `db:"created_on"`
	ModifiedOn_ time.Time `db:"modified_on"`

	CreatedBy_  UserID `db:"created_by_id"`
	ModifiedBy_ UserID `db:"modified_by_id"`

	IsNew_ bool
}

// UUID returns the UUID for this contact
func (c *Contact) UUID() ContactUUID { return c.UUID_ }

const sqlInsertContact = `
INSERT INTO 
	contacts_contact(org_id, is_active, status, uuid, created_on, modified_on, created_by_id, modified_by_id, name, ticket_count) 
              VALUES(:org_id, TRUE, 'A', :uuid, :created_on, :modified_on, :created_by_id, :modified_by_id, :name, 0)
RETURNING id
`

// InsertContact inserts the passed in contact, the id field will be populated with the result on success
func InsertContact(ctx context.Context, tx *sqlx.Tx, contact *Contact) error {
	// see https://github.com/jmoiron/sqlx/issues/447
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
