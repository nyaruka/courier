package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
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

	URNID_ ContactURNID   `db:"urn_id"` // the URN the contact was resolved by
	URNs_  pq.StringArray `db:"urns"`   // identities of all the contact's URNs

	CreatedOn_  time.Time `db:"created_on"`
	ModifiedOn_ time.Time `db:"modified_on"`

	CreatedBy_  UserID `db:"created_by_id"`
	ModifiedBy_ UserID `db:"modified_by_id"`

	// set when resolved for an incoming message
	IsNew_        bool       // created for this message
	URNsToAppend_ []urns.URN // other URNs of the sender for mailroom to append (see appendableURNs)
}

// UUID returns the UUID for this contact
func (c *Contact) UUID() ContactUUID { return c.UUID_ }

const sqlInsertContact = `
INSERT INTO 
	contacts_contact(org_id, is_active, status, uuid, created_on, modified_on, created_by_id, modified_by_id, name) 
              VALUES(:org_id, TRUE, 'A', :uuid, :created_on, :modified_on, :created_by_id, :modified_by_id, :name)
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
	u.id as "urn_id",
	ARRAY(SELECT identity FROM contacts_contacturn WHERE contact_id = c.id) AS "urns"
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

	// didn't find it, we need to create it instead - if the creation check allows it
	if err := ContactCreationCheck(ctx, rt, channel, urn, clog); err != nil {
		return nil, err
	}

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
	contact.URNs_ = pq.StringArray{string(urn.Identity())}

	rt.Stats.RecordContactCreated()
	contactCreated(org)

	return contact, nil
}

const sqlSelectOrgContactCount = `
SELECT COALESCE(SUM(c.count), 0)
  FROM contacts_contactgroup g
 INNER JOIN contacts_contactgroupcount c ON c.group_id = g.id
 WHERE g.org_id = $1 AND g.group_type IN ('A', 'B', 'S', 'V')`

// gets the total number of contacts in the given org by summing the squashed counts of its status groups, which are
// maintained by database triggers. Counts are stored as deltas which are periodically squashed so we have to sum them.
func getOrgContactCount(ctx context.Context, rt *runtime.Runtime, orgID OrgID) (int, error) {
	var count int
	if err := rt.DB.GetContext(ctx, &count, sqlSelectOrgContactCount, orgID); err != nil {
		return 0, fmt.Errorf("error getting contact count for org #%d: %w", orgID, err)
	}
	return count, nil
}

// orgContactLimit returns the effective contact limit for the org which owns the given channel, which is the
// org's own limit if it has one, and otherwise the configured default. Zero or less means no limit.
func orgContactLimit(rt *runtime.Runtime, channel *Channel) int {
	limit := channel.OrgContactLimit()
	if limit == NoLimit {
		limit = rt.Config.DefaultContactLimit
	}
	return limit
}

// ContactCreationCheck is called before a contact is created for a URN received on a channel, and can refuse the
// creation by returning a LimitReachedError - which callers treat as a refusal to be reported rather than a failure
// to be retried, and which should be recorded on the channel log so that the workspace can see it. The default
// enforces the workspace's contact limit. Deployments can replace it to apply their own policies as well, e.g. from
// main before starting the service.
var ContactCreationCheck = CheckContactLimit

// CheckContactLimit is the default contact creation check: it refuses the creation if the workspace which owns the
// given channel has reached its contact limit, recording that on the channel log.
func CheckContactLimit(ctx context.Context, rt *runtime.Runtime, channel *Channel, urn urns.URN, clog *ChannelLog) error {
	limit := orgContactLimit(rt, channel)
	if limit <= 0 {
		return nil
	}

	count, err := contactCounts.GetOrFetch(ctx, channel.OrgID())
	if err != nil {
		return fmt.Errorf("error getting contact count: %w", err)
	}

	if count.Load() >= int64(limit) {
		clog.Error(ErrorContactLimitReached(limit))
		return &LimitReachedError{Limit: "contacts", Max: limit}
	}

	return nil
}

// FlushContactCounts clears the cached contact counts - used in tests after contacts or their counts are modified
// in the database. It's a no-op if the cache hasn't been created, i.e. Start hasn't been called.
func FlushContactCounts() {
	if contactCounts != nil {
		contactCounts.Clear()
	}
}

// records that a contact was created in the given org so that our cached count doesn't let a burst of new contacts
// run away past the org's limit before the count is next reloaded. Only orgs whose count is already cached - i.e.
// those with a limit being enforced - are updated, as the count is only ever read for those.
func contactCreated(orgID OrgID) {
	if count := contactCounts.Get(orgID); count != nil {
		count.Add(1)
	}
}

// contactForMsg resolves the contact for an incoming message: the one owning its URN, or failing that one owning any
// of the other URNs the handler reported for the sender - so a contact created from WhatsApp messages received with
// only a business-scoped user ID (see https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/)
// is reused rather than duplicated when the phone number appears. Otherwise a new contact is created. The other URNs
// are left for mailroom to append where that will succeed (see appendableURNs).
func contactForMsg(ctx context.Context, rt *runtime.Runtime, m *MsgIn, clog *ChannelLog) (*Contact, error) {
	org, channel := m.Channel_.OrgID(), m.Channel_

	// simple case: no other URNs to consider, look up or create by the primary URN
	if len(m.OtherURNs_) == 0 {
		return contactForURN(ctx, rt, org, channel, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
	}

	// try the primary URN first, without creating a contact
	contact, err := contactForURN(ctx, rt, org, channel, m.URN_, m.URNAuthTokens_, m.ContactName_, false, clog)
	if err != nil {
		return nil, err
	}

	// the primary URN didn't match an existing contact, try the others
	for _, other := range m.OtherURNs_ {
		if contact != nil {
			break
		}
		contact, err = contactForURN(ctx, rt, org, channel, other, nil, "", false, clog)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			// add the primary URN to it so the message stays attributed to the primary URN
			cu, added, err := claimURN(ctx, rt, channel, contact, m.URN_, m.URNAuthTokens_)
			if err != nil {
				return nil, err
			}
			if !added {
				// the primary URN was created for another contact while we were looking, start over with a lookup
				return contactForMsg(ctx, rt, m, clog)
			}
			contact.URNID_ = cu.ID
			contact.URNs_ = append(contact.URNs_, string(m.URN_.Identity()))
			rt.Stats.RecordContactMatchedByOtherURN()
		}
	}

	// no existing contact matched any URN, create one from the primary URN
	if contact == nil {
		contact, err = contactForURN(ctx, rt, org, channel, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
		if err != nil {
			return nil, err
		}
	}

	contact.URNsToAppend_, err = appendableContactURNs(ctx, rt, contact, m.OtherURNs_)
	if err != nil {
		return nil, err
	}

	return contact, nil
}

const sqlSelectURNOwners = `
SELECT u.org_id, u.identity, u.contact_id,
       NOT EXISTS(SELECT 1 FROM contacts_contacturn o WHERE o.contact_id = u.contact_id AND o.id != u.id) AS is_shell
  FROM contacts_contacturn u
 WHERE (u.org_id, u.identity) IN (VALUES(:org_id::int, :identity))`

// urnKey identifies a URN within an org
type urnKey struct {
	OrgID    OrgID  `db:"org_id"`
	Identity string `db:"identity"`
}

// urnOwner is who owns a URN, if anyone
type urnOwner struct {
	urnKey
	ContactID ContactID `db:"contact_id"` // nil if the URN is orphaned
	IsShell   bool      `db:"is_shell"`   // the owner has no other URNs
}

// lookupURNOwners looks up the owners of the given URNs, in one query so that a batch of statuses costs one
func lookupURNOwners(ctx context.Context, rt *runtime.Runtime, keys []urnKey) (map[urnKey]*urnOwner, error) {
	query, args, err := dbutil.BulkSQL(rt.DB, sqlSelectURNOwners, keys)
	if err != nil {
		return nil, err
	}
	rows, err := rt.DB.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error looking up URN owners: %w", err)
	}
	defer rows.Close()

	owners := make(map[urnKey]*urnOwner, len(keys))
	for rows.Next() {
		o := &urnOwner{}
		if err := rows.StructScan(o); err != nil {
			return nil, fmt.Errorf("error scanning URN owner: %w", err)
		}
		owners[o.urnKey] = o
	}
	return owners, rows.Err()
}

// appendableURNs returns which of the given URNs are worth asking mailroom to append to the contact: those nobody
// owns, and WhatsApp BSUIDs owned by a shell contact - one created from messages received with only the BSUID -
// which mailroom reassigns. A URN owned by any other contact is a duplicate contact we can't resolve, and mailroom
// would refuse the append, so it's left where it is and counted rather than requeued on every message, send and
// status. Owners are as looked up by lookupURNOwners.
func appendableURNs(rt *runtime.Runtime, contact *Contact, others []urns.URN, owners map[urnKey]*urnOwner) []urns.URN {
	var toAppend []urns.URN

	for _, u := range others {
		owner := owners[urnKey{contact.OrgID_, string(u.Identity())}]

		switch {
		case owner != nil && owner.ContactID == contact.ID_: // already ours
		case owner == nil || owner.ContactID == NilContactID || (owner.IsShell && urns.IsWhatsAppBSUID(u)):
			toAppend = append(toAppend, u)
			rt.Stats.RecordURNOutcome(true)
		default:
			slog.Info("URN not appended to contact because it belongs to another contact", "urn", u, "contact", contact.UUID_, "owner_id", owner.ContactID)
			rt.Stats.RecordURNOutcome(false)
		}
	}

	return toAppend
}

// looks up the owners of the given URNs and returns those to ask mailroom to append (see appendableURNs). URNs the
// contact is already known to have - the common case for a contact loaded with its URNs - aren't looked up.
func appendableContactURNs(ctx context.Context, rt *runtime.Runtime, contact *Contact, others []urns.URN) ([]urns.URN, error) {
	unknown := make([]urns.URN, 0, len(others))
	keys := make([]urnKey, 0, len(others))
	for _, u := range others {
		if !slices.Contains(contact.URNs_, string(u.Identity())) {
			unknown = append(unknown, u)
			keys = append(keys, urnKey{contact.OrgID_, string(u.Identity())})
		}
	}
	if len(unknown) == 0 {
		return nil, nil
	}

	owners, err := lookupURNOwners(ctx, rt, keys)
	if err != nil {
		return nil, err
	}
	return appendableURNs(rt, contact, unknown, owners), nil
}

// claimURN adds the given URN to the contact if not already present. Returns false without adding if the URN belongs
// to another contact or was concurrently inserted by another writer, rather than stealing it.
func claimURN(ctx context.Context, rt *runtime.Runtime, channel *Channel, contact *Contact, urn urns.URN, authTokens map[string]string) (*ContactURN, bool, error) {
	tx, err := rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("error beginning transaction: %w", err)
	}

	contactURN, err := GetOrCreateContactURN(ctx, tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()

		// the URN was inserted by someone else after we tried to look it up
		if dbutil.IsUniqueViolation(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("error adding URN to contact: %w", err)
	}

	// the URN was created for another contact while we were looking, roll back rather than steal it
	if contactURN.PrevContactID != NilContactID {
		tx.Rollback()
		return nil, false, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("error committing transaction: %w", err)
	}

	return contactURN, true, nil
}
