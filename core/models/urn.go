package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null/v3"
	"github.com/vinovest/sqlx"
)

// ContactURNID represents a contact urn's id
type ContactURNID null.Int

// NilContactURNID is our constant for a nil contact URN id
const NilContactURNID = ContactURNID(0)

func (i *ContactURNID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ContactURNID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ContactURNID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ContactURNID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// ContactURN is our struct to map to database level URNs
type ContactURN struct {
	ID            ContactURNID     `db:"id"`
	OrgID         OrgID            `db:"org_id"`
	ContactID     ContactID        `db:"contact_id"`
	Identity      string           `db:"identity"`
	Scheme        string           `db:"scheme"`
	Path          string           `db:"path"`
	Display       null.String      `db:"display"`
	AuthTokens    null.Map[string] `db:"auth_tokens"`
	Priority      int              `db:"priority"`
	ChannelID     ChannelID        `db:"channel_id"`
	PrevContactID ContactID
}

// NewContactURN returns a new URN for the passed in org, contact and string URN
func NewContactURN(org OrgID, channelID ChannelID, contactID ContactID, urn urns.URN, authTokens map[string]string) *ContactURN {
	return &ContactURN{
		OrgID:      org,
		ChannelID:  channelID,
		ContactID:  contactID,
		Identity:   string(urn.Identity()),
		Scheme:     urn.Scheme(),
		Path:       urn.Path(),
		Display:    null.String(urn.Display()),
		AuthTokens: null.Map[string](authTokens),
	}
}

const sqlSelectURNsByContact = `
  SELECT id, org_id, contact_id, identity, scheme, path, display, auth_tokens, priority, channel_id
    FROM contacts_contacturn
   WHERE contact_id = $1
ORDER BY priority DESC`

const sqlSelectURNByIdentity = `
  SELECT id, org_id, contact_id, identity, scheme, path, display, auth_tokens, priority, channel_id
    FROM contacts_contacturn
   WHERE org_id = $1 AND identity = $2
ORDER BY priority DESC 
   LIMIT 1`

// GetURNsForContact returns all the URNs for the passed in contact, sorted by priority
func GetURNsForContact(ctx context.Context, db *sqlx.Tx, contactID ContactID) ([]*ContactURN, error) {
	// select all the URNs for this contact
	rows, err := db.QueryxContext(ctx, sqlSelectURNsByContact, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	urns := make([]*ContactURN, 0, 3)

	for rows.Next() {
		u := &ContactURN{}

		if err := rows.StructScan(u); err != nil {
			return nil, err
		}

		urns = append(urns, u)
	}
	return urns, nil
}

// UpdateContactURNMetadata updates channel, display and auth tokens on the URN for the given
// contact without modifying priorities (priority reordering is delegated to mailroom)
func UpdateContactURNMetadata(ctx context.Context, db *sqlx.Tx, channel *Channel, contact *Contact, urn urns.URN, authTokens map[string]string) error {
	contactURN, err := GetContactURNByIdentity(ctx, db, channel.OrgID(), urn)
	if err != nil {
		return fmt.Errorf("error getting URN: %w", err)
	}

	display := null.String(urn.Display())
	needsUpdate := false

	if channel.HasRole(ChannelRoleSend) && contactURN.ChannelID != channel.ID() {
		contactURN.ChannelID = channel.ID()
		needsUpdate = true
	}
	if contactURN.Display != display {
		contactURN.Display = display
		needsUpdate = true
	}
	if authTokens != nil && !utils.MapContains(contactURN.AuthTokens, authTokens) {
		utils.MapUpdate(contactURN.AuthTokens, authTokens)
		needsUpdate = true
	}

	if needsUpdate {
		return UpdateContactURN(ctx, db, contactURN)
	}
	return nil
}

// GetContactURNByIdentity returns the URN for the passed in org and identity
func GetContactURNByIdentity(ctx context.Context, db *sqlx.Tx, org OrgID, urn urns.URN) (*ContactURN, error) {
	contactURN := NewContactURN(org, NilChannelID, NilContactID, urn, map[string]string{})
	err := db.GetContext(ctx, contactURN, sqlSelectURNByIdentity, org, urn.Identity())
	if err != nil {
		return nil, err
	}
	return contactURN, nil
}

// GetOrCreateContactURN returns the URN for the passed in org and URN, creating and associating
// it with the passed in contact if necessary
func GetOrCreateContactURN(ctx context.Context, db *sqlx.Tx, channel *Channel, contactID ContactID, urn urns.URN, authTokens map[string]string) (*ContactURN, error) {
	contactURN := NewContactURN(channel.OrgID(), NilChannelID, contactID, urn, authTokens)
	if channel.HasRole(ChannelRoleSend) {
		contactURN.ChannelID = channel.ID()
	}
	err := db.GetContext(ctx, contactURN, sqlSelectURNByIdentity, channel.OrgID(), urn.Identity())
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error looking up URN by identity: %w", err)
	}

	// we didn't find it, let's insert it
	if err == sql.ErrNoRows {
		err = InsertContactURN(ctx, db, contactURN)
		if err != nil {
			return nil, fmt.Errorf("error inserting URN: %w", err)
		}
	}

	display := null.String(urn.Display())

	// make sure our contact URN is up to date
	if (channel.HasRole(ChannelRoleSend) && contactURN.ChannelID != channel.ID()) || contactURN.ContactID != contactID || contactURN.Display != display {
		contactURN.PrevContactID = contactURN.ContactID
		if channel.HasRole(ChannelRoleSend) {
			contactURN.ChannelID = channel.ID()
		}
		contactURN.ContactID = contactID
		contactURN.Display = display
		err = UpdateContactURN(ctx, db, contactURN)
		if err != nil {
			return nil, fmt.Errorf("error updating URN: %w", err)
		}
	}

	// update our auth tokens if any provided
	if authTokens != nil {
		utils.MapUpdate(contactURN.AuthTokens, authTokens)

		err = UpdateContactURN(ctx, db, contactURN)
	}
	if err != nil {
		return contactURN, fmt.Errorf("error updating URN auth: %w", err)
	}
	return contactURN, nil
}

const sqlInsertURN = `
INSERT INTO contacts_contacturn(org_id, identity, path, scheme, display, auth_tokens, priority, channel_id, contact_id)
                         VALUES(:org_id, :identity, :path, :scheme, :display, :auth_tokens, :priority, :channel_id, :contact_id)
  RETURNING id`

// InsertContactURN inserts the passed in urn, the id field will be populated with the result on success
func InsertContactURN(ctx context.Context, tx *sqlx.Tx, urn *ContactURN) error {
	// see https://github.com/vinovest/sqlx/issues/447
	rows, err := tx.NamedQuery(sqlInsertURN, urn)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&urn.ID)
	}
	return err
}

const sqlUpdateURN = `
UPDATE contacts_contacturn
   SET channel_id = :channel_id, contact_id = :contact_id, display = :display, auth_tokens = :auth_tokens, priority = :priority
 WHERE id = :id`

const sqlFullyUpdateURN = `
UPDATE contacts_contacturn
   SET channel_id = :channel_id, contact_id = :contact_id, identity = :identity, path = :path, display = :display, auth_tokens = :auth_tokens, priority = :priority
 WHERE id = :id`

// UpdateContactURN updates the channel and contact on an existing URN
func UpdateContactURN(ctx context.Context, tx *sqlx.Tx, urn *ContactURN) error {
	// see https://github.com/vinovest/sqlx/issues/447
	rows, err := tx.NamedQuery(sqlUpdateURN, urn)
	if err != nil {
		slog.Error("error updating contact urn", "error", err, "urn_id", urn.ID)
		return err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&urn.ID)
	}
	return err
}

// UpdateContactURNFully updates the identity, channel and contact on an existing URN
func UpdateContactURNFully(ctx context.Context, tx *sqlx.Tx, urn *ContactURN) error {
	// see https://github.com/vinovest/sqlx/issues/447
	rows, err := tx.NamedQuery(sqlFullyUpdateURN, urn)
	if err != nil {
		slog.Error("error updating contact urn", "error", err, "urn_id", urn.ID)
		return err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&urn.ID)
	}
	return err
}
