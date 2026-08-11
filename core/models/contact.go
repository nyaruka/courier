package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
	"github.com/vinovest/sqlx"
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

// URNDescriber is the interface handlers which can look up URN metadata for new contacts should satisfy.
type URNDescriber interface {
	DescribeURN(context.Context, *Channel, urns.URN, *ChannelLog) (map[string]string, error)
}

var urnDescribers = make(map[ChannelType]URNDescriber)

// RegisterURNDescriber registers a URN describer for the given channel type - called for handlers that
// implement URNDescriber when they are registered.
func RegisterURNDescriber(typ ChannelType, d URNDescriber) {
	urnDescribers[typ] = d
}

// used by unit tests to slow down urn operations to test races
var urnSleep bool

const sqlLookupContactFromURN = `
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

// GetContact returns (or creates) the contact for the passed in channel and URN
func GetContact(ctx context.Context, rt *runtime.Runtime, channel *Channel, urn urns.URN, authTokens map[string]string, name string, allowCreate bool, clog *ChannelLog) (*Contact, error) {
	return contactForURN(ctx, rt, channel.OrgID_, channel, urn, authTokens, name, allowCreate, clog)
}

// contactForURN first tries to look up a contact for the passed in URN, if not finding one then creating one
func contactForURN(ctx context.Context, rt *runtime.Runtime, org OrgID, channel *Channel, urn urns.URN, authTokens map[string]string, name string, allowCreate bool, clog *ChannelLog) (*Contact, error) {
	log := slog.With("org_id", org, "urn", urn.Identity(), "channel_uuid", channel.UUID(), "log_uuid", clog.UUID)

	// try to look up our contact by URN
	contact := &Contact{}
	err := rt.DB.GetContext(ctx, contact, sqlLookupContactFromURN, urn.Identity(), org)
	if err != nil && err != sql.ErrNoRows {
		log.Error("error looking up contact by URN", "error", err)
		return nil, fmt.Errorf("error looking up contact by URN: %w", err)
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		tx, err := rt.DB.BeginTxx(ctx, nil)
		if err != nil {
			log.Error("error beginning transaction", "error", err)
			return nil, fmt.Errorf("error beginning transaction: %w", err)
		}

		// update channel, display and auth tokens on the URN (priority reordering is delegated to mailroom)
		err = UpdateContactURNMetadata(ctx, tx, channel, contact, urn, authTokens)
		if err != nil {
			log.Error("error updating URN metadata for contact", "error", err)
			tx.Rollback()
			return nil, fmt.Errorf("error updating URN metadata for contact: %w", err)
		}
		return contact, tx.Commit()
	}

	if !allowCreate {
		return nil, nil
	}

	// didn't find it, we need to create it instead
	contact.OrgID_ = org
	contact.UUID_ = ContactUUID(uuids.NewV4())
	contact.CreatedOn_ = time.Now()
	contact.CreatedBy_ = systemUserID
	contact.ModifiedOn_ = time.Now()
	contact.ModifiedBy_ = systemUserID
	contact.IsNew_ = true

	// if we aren't an anonymous org, we want to look up a name if possible and set it
	if !channel.OrgIsAnon() {
		// no name was passed in, see if our handler can look up information for this URN
		if name == "" {
			describer := urnDescribers[channel.ChannelType()]
			if describer != nil {
				attrs, err := describer.DescribeURN(ctx, channel, urn, clog)

				// in the case of errors, we log the error but move onwards anyways
				if err != nil {
					log.Error("unable to describe URN", "error", err)
				} else {
					name = attrs["name"]
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
	tx, err := rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %w", err)
	}

	if err := InsertContact(ctx, tx, contact); err != nil {
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
	contactURN, err := GetOrCreateContactURN(ctx, tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()

		if dbutil.IsUniqueViolation(err) {
			// if this was a duplicate URN, start over with a contact lookup
			return contactForURN(ctx, rt, org, channel, urn, authTokens, name, true, clog)
		}
		return nil, fmt.Errorf("error getting URN for contact: %w", err)
	}

	// we stole the URN from another contact, roll back and start over
	if contactURN.PrevContactID != NilContactID {
		tx.Rollback()
		return contactForURN(ctx, rt, org, channel, urn, authTokens, name, true, clog)
	}

	// all is well, we created the new contact, commit and move forward
	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("error commiting transaction: %w", err)
	}

	// store this URN on our contact
	contact.URNID_ = contactURN.ID

	rt.Stats.RecordContactCreated()

	return contact, nil
}

// contactForMsg resolves the contact for an incoming message. Normally that's a lookup by (or creation from) the
// message's primary URN. But when a WhatsApp message arrives with a phone number as its primary URN and a
// business-scoped user ID attached as its new URN, and the phone number doesn't match an existing contact, we also
// look for one by the BSUID - so a contact created from messages received while the user's phone number was omitted
// (see https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/) is
// reused rather than duplicated. In that case the phone number is added to the matched contact so the message is
// still attributed to it as the primary URN.
func contactForMsg(ctx context.Context, rt *runtime.Runtime, m *MsgIn, clog *ChannelLog) (*Contact, error) {
	altURN := altLookupURN(m)

	// simple case: no alternative URN to consider, look up or create by the primary URN
	if altURN == urns.NilURN {
		return contactForURN(ctx, rt, m.Channel_.OrgID(), m.Channel_, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
	}

	// try the primary URN first, without creating a contact
	contact, err := contactForURN(ctx, rt, m.Channel_.OrgID(), m.Channel_, m.URN_, m.URNAuthTokens_, m.ContactName_, false, clog)
	if err != nil || contact != nil {
		return contact, err
	}

	// the primary URN didn't match an existing contact, try the alternative
	contact, err = contactForURN(ctx, rt, m.Channel_.OrgID(), m.Channel_, altURN, nil, "", false, clog)
	if err != nil {
		return nil, err
	}
	if contact != nil {
		// matched an existing contact by the BSUID - add the primary URN to it so the message stays attributed to
		// the primary URN
		added, err := addContactURN(ctx, rt, m.Channel_, contact, m.URN_, m.URNAuthTokens_)
		if err != nil {
			return nil, err
		}
		if !added {
			// the primary URN was created for another contact while we were looking, start over with a lookup
			return contactForMsg(ctx, rt, m, clog)
		}
		return contact, nil
	}

	// no existing contact matched either URN, create one from the primary URN
	return contactForURN(ctx, rt, m.Channel_.OrgID(), m.Channel_, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
}

// altLookupURN returns an alternative URN to look up an existing contact by when the message's primary URN doesn't
// match one: for a message from a WhatsApp phone number with a business-scoped user ID attached as its new URN,
// that's the BSUID.
func altLookupURN(m *MsgIn) urns.URN {
	if m.NewURN_ != nil && m.URN_.Scheme() == urns.WhatsApp.Prefix && urns.IsWhatsAppBSUID(m.NewURN_.Value) {
		return m.NewURN_.Value
	}
	return urns.NilURN
}

// addContactURN adds the given URN to the contact (if not already present) and points the contact's URNID at it, so
// an incoming message is attributed to this URN. Returns false without adding if the URN belongs to another contact
// or was concurrently inserted by another writer, rather than stealing it - the caller should restart its lookup.
func addContactURN(ctx context.Context, rt *runtime.Runtime, channel *Channel, contact *Contact, urn urns.URN, authTokens map[string]string) (bool, error) {
	tx, err := rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("error beginning transaction: %w", err)
	}

	contactURN, err := GetOrCreateContactURN(ctx, tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()

		// the URN was inserted by someone else after we tried to look it up
		if dbutil.IsUniqueViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("error adding URN to contact: %w", err)
	}

	// the URN was created for another contact while we were looking, roll back rather than steal it
	if contactURN.PrevContactID != NilContactID {
		tx.Rollback()
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("error committing transaction: %w", err)
	}

	contact.URNID_ = contactURN.ID
	return true, nil
}
