package rapidpro

import (
	"database/sql"
	"database/sql/driver"
	"fmt"

	"github.com/nyaruka/null"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
)

// ContactURNID represents a contact urn's id
type ContactURNID null.Int

// NilContactURNID is our constant for a nil contact URN id
const NilContactURNID = ContactURNID(0)

// MarshalJSON marshals into JSON. 0 values will become null
func (i ContactURNID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

// UnmarshalJSON unmarshals from JSON. null values become 0
func (i *ContactURNID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

// Value returns the db value, null is returned for 0
func (i ContactURNID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

// Scan scans from the db value. null values become 0
func (i *ContactURNID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// NewDBContactURN returns a new ContactURN object for the passed in org, contact and string urn, this is not saved to the DB yet
func newDBContactURN(org OrgID, channelID courier.ChannelID, contactID ContactID, urn urns.URN, auth string) *DBContactURN {
	return &DBContactURN{
		OrgID:     org,
		ChannelID: channelID,
		ContactID: contactID,
		Identity:  string(urn.Identity()),
		Scheme:    urn.Scheme(),
		Path:      urn.Path(),
		Display:   null.String(urn.Display()),
		Auth:      null.String(auth),
	}
}

const selectContactURNs = `
SELECT 
	id, 
	identity, 
	scheme, 
	display, 
	auth, 
	priority, 
	contact_id, 
	channel_id
FROM 
	contacts_contacturn
WHERE 
	contact_id = $1
ORDER BY 
	priority desc
`

// selectContactURNs returns all the ContactURNs for the passed in contact, sorted by priority
func contactURNsForContact(db *sqlx.Tx, contactID ContactID) ([]*DBContactURN, error) {
	// select all the URNs for this contact
	rows, err := db.Queryx(selectContactURNs, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// read our URNs out
	urns := make([]*DBContactURN, 0, 3)
	idx := 0
	for rows.Next() {
		u := &DBContactURN{}
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		urns = append(urns, u)
		idx++
	}
	return urns, nil
}

// setDefaultURN makes sure that the passed in URN is the default URN for this contact and
// that the passed in channel is the default one for that URN
//
// Note that the URN must be one of the contact's URN before calling this method
func setDefaultURN(db *sqlx.Tx, channelID courier.ChannelID, contact *DBContact, urn urns.URN, auth string) error {
	scheme := urn.Scheme()
	contactURNs, err := contactURNsForContact(db, contact.ID_)
	if err != nil {
		logrus.WithError(err).WithField("urn", urn.Identity()).WithField("channel_id", channelID).Error("error looking up contact urns")
		return err
	}

	// no URNs? that's an error
	if len(contactURNs) == 0 {
		return fmt.Errorf("URN '%s' not present for contact %d", urn.Identity(), contact.ID_)
	}

	// only a single URN and it is ours
	if contactURNs[0].Identity == string(urn.Identity()) {
		display := urn.Display()

		// if display, channel id or auth changed, update them
		if string(contactURNs[0].Display) != display || contactURNs[0].ChannelID != channelID || (auth != "" && string(contactURNs[0].Auth) != auth) {
			contactURNs[0].Display = null.String(display)
			contactURNs[0].ChannelID = channelID
			if auth != "" {
				contactURNs[0].Auth = null.String(auth)
			}
			return updateContactURN(db, contactURNs[0])
		}
		return nil
	}

	// multiple URNs and we aren't the top, iterate across them and update channel for matching schemes
	// this is kinda expensive (n SQL queries) but only happens for cases where there are multiple URNs for a contact (rare) and
	// the preferred channel changes (rare as well)
	topPriority := 99
	currPriority := 50
	for _, existing := range contactURNs {
		// if this is current URN, make sure it has an updated auth as well
		if existing.Identity == string(urn.Identity()) {
			existing.Priority = topPriority
			existing.ChannelID = channelID
			if auth != "" {
				existing.Auth = null.String(auth)
			}
		} else {
			existing.Priority = currPriority

			// if this is a phone number and we just received a message on a tel scheme, set that as our new preferred channel
			if existing.Scheme == urns.TelScheme && scheme == urns.TelScheme {
				existing.ChannelID = channelID
			}
			currPriority--
		}
		err := updateContactURN(db, existing)
		if err != nil {
			return err
		}
	}

	return nil
}

const selectOrgURN = `
SELECT 
	org_id, 
	id, 
	identity, 
	scheme, 
	path, 
	display, 
	auth, 
	priority, 
	channel_id, 
	contact_id 
FROM 
	contacts_contacturn
WHERE 
	org_id = $1 AND 
	identity = $2
ORDER BY 
	priority desc 
LIMIT 1
`

// contactURNForURN returns the ContactURN for the passed in org and URN, creating and associating
// it with the passed in contact if necessary
func contactURNForURN(db *sqlx.Tx, org OrgID, channelID courier.ChannelID, contactID ContactID, urn urns.URN, auth string) (*DBContactURN, error) {
	contactURN := newDBContactURN(org, channelID, contactID, urn, auth)
	err := db.Get(contactURN, selectOrgURN, org, urn.Identity())
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

	display := null.String(urn.Display())

	// make sure our contact URN is up to date
	if contactURN.ChannelID != channelID || contactURN.ContactID != contactID || contactURN.Display != display {
		contactURN.PrevContactID = contactURN.ContactID
		contactURN.ChannelID = channelID
		contactURN.ContactID = contactID
		contactURN.Display = display
		err = updateContactURN(db, contactURN)
		if err != nil {
			return nil, err
		}
	}

	// update our auth if we have a value set
	if auth != "" && auth != string(contactURN.Auth) {
		contactURN.Auth = null.String(auth)
		err = updateContactURN(db, contactURN)
	}

	return contactURN, err
}

const insertURN = `
INSERT INTO 
	contacts_contacturn(org_id, identity, path, scheme, display, auth, priority, channel_id, contact_id)
                 VALUES(:org_id, :identity, :path, :scheme, :display, :auth, :priority, :channel_id, :contact_id)
RETURNING id
`

// InsertContactURN inserts the passed in urn, the id field will be populated with the result on success
func insertContactURN(db *sqlx.Tx, urn *DBContactURN) error {
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
UPDATE 
	contacts_contacturn
SET 
	channel_id = :channel_id, 
	contact_id = :contact_id, 
	display = :display, 
	auth = :auth, 
	priority = :priority
WHERE 
	id = :id
`

// UpdateContactURN updates the Channel and Contact on an existing URN
func updateContactURN(db *sqlx.Tx, urn *DBContactURN) error {
	rows, err := db.NamedQuery(updateURN, urn)
	if err != nil {
		logrus.WithError(err).WithField("urn_id", urn.ID).Error("error updating contact urn")
		return err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&urn.ID)
	}
	return err
}

// DBContactURN is our struct to map to database level URNs
type DBContactURN struct {
	OrgID         OrgID             `db:"org_id"`
	ID            ContactURNID      `db:"id"`
	Identity      string            `db:"identity"`
	Scheme        string            `db:"scheme"`
	Path          string            `db:"path"`
	Display       null.String       `db:"display"`
	Auth          null.String       `db:"auth"`
	Priority      int               `db:"priority"`
	ChannelID     courier.ChannelID `db:"channel_id"`
	ContactID     ContactID         `db:"contact_id"`
	PrevContactID ContactID
}
