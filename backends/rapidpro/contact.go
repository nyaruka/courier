package rapidpro

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/dbutil"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// used by unit tests to slow down urn operations to test races
var urnSleep bool

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
func contactForURN(ctx context.Context, b *backend, org models.OrgID, channel *models.Channel, urn urns.URN, authTokens map[string]string, name string, allowCreate bool, clog *courier.ChannelLog) (*models.Contact, error) {
	log := slog.With("org_id", org, "urn", urn.Identity(), "channel_uuid", channel.UUID(), "log_uuid", clog.UUID)

	// try to look up our contact by URN
	contact := &models.Contact{}
	err := b.rt.DB.GetContext(ctx, contact, lookupContactFromURNSQL, urn.Identity(), org)
	if err != nil && err != sql.ErrNoRows {
		log.Error("error looking up contact by URN", "error", err)
		return nil, fmt.Errorf("error looking up contact by URN: %w", err)
	}

	// we found it, return it
	if err != sql.ErrNoRows {
		tx, err := b.rt.DB.BeginTxx(ctx, nil)
		if err != nil {
			log.Error("error beginning transaction", "error", err)
			return nil, fmt.Errorf("error beginning transaction: %w", err)
		}

		// update channel, display and auth tokens on the URN (priority reordering is delegated to mailroom)
		err = models.UpdateContactURNMetadata(ctx, tx, channel, contact, urn, authTokens)
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
	contact.UUID_ = models.ContactUUID(uuids.NewV4())
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
	tx, err := b.rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error beginning transaction: %w", err)
	}

	if err := models.InsertContact(ctx, tx, contact); err != nil {
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
	contactURN, err := models.GetOrCreateContactURN(ctx, tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()

		if dbutil.IsUniqueViolation(err) {
			// if this was a duplicate URN, start over with a contact lookup
			return contactForURN(ctx, b, org, channel, urn, authTokens, name, true, clog)
		}
		return nil, fmt.Errorf("error getting URN for contact: %w", err)
	}

	// we stole the URN from another contact, roll back and start over
	if contactURN.PrevContactID != models.NilContactID {
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

// contactForMsg resolves the contact for an incoming message. It normally looks the contact up by (or creates it
// from) the message's primary URN. But when a WhatsApp message arrives with a business-scoped user ID as its
// primary URN and a phone number attached as its new URN, we also look for an existing contact by that phone
// number - in both its legacy all-digit whatsapp form and its tel form - so a contact which predates the BSUID is
// reused rather than duplicated. In that case the BSUID is added to the matched contact so it stays the message's
// (highest priority) URN.
func contactForMsg(ctx context.Context, b *backend, m *MsgIn, clog *courier.ChannelLog) (*models.Contact, error) {
	altURNs := altLookupURNs(m)

	// simple case: no alternative URNs to consider, look up or create by the primary URN
	if len(altURNs) == 0 {
		return contactForURN(ctx, b, m.OrgID_, m.channel, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
	}

	// try the primary URN first, without creating a contact
	contact, err := contactForURN(ctx, b, m.OrgID_, m.channel, m.URN_, m.URNAuthTokens_, m.ContactName_, false, clog)
	if err != nil {
		return nil, err
	}
	if contact != nil {
		return contact, nil
	}

	// the primary URN didn't match an existing contact, try the alternatives
	for _, alt := range altURNs {
		contact, err = contactForURN(ctx, b, m.OrgID_, m.channel, alt, nil, "", false, clog)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			// matched an existing contact by an alternative URN - add the primary URN to it so the message
			// stays attributed to the primary URN
			if err := addContactURN(ctx, b, m.channel, contact, m.URN_, m.URNAuthTokens_); err != nil {
				return nil, err
			}
			return contact, nil
		}
	}

	// no existing contact matched any URN, create one from the primary URN
	return contactForURN(ctx, b, m.OrgID_, m.channel, m.URN_, m.URNAuthTokens_, m.ContactName_, true, clog)
}

// altLookupURNs returns alternative URNs to look up an existing contact by when the message's primary URN doesn't
// match one. For a WhatsApp business-scoped user ID (a whatsapp URN in the CC.xxx form) with a phone number
// attached as its new URN, those are the phone number in its legacy all-digit whatsapp form and its tel form.
func altLookupURNs(m *MsgIn) []urns.URN {
	if m.NewURN_ == nil || m.URN_.Scheme() != urns.WhatsApp.Prefix {
		return nil
	}

	// only business-scoped user IDs (which contain a "."), not all-digit whatsapp phone numbers
	if !strings.Contains(m.URN_.Path(), ".") {
		return nil
	}

	phone := m.NewURN_.Value
	if phone.Scheme() != urns.Phone.Prefix {
		return nil
	}
	digits := strings.TrimPrefix(phone.Path(), "+")

	var altURNs []urns.URN
	if waURN, err := urns.New(urns.WhatsApp, digits); err == nil {
		altURNs = append(altURNs, waURN)
	}
	return append(altURNs, phone)
}

// addContactURN adds the given URN to the contact (if not already present) and points the contact's URNID at it,
// so the incoming message is attributed to this URN.
func addContactURN(ctx context.Context, b *backend, channel *models.Channel, contact *models.Contact, urn urns.URN, authTokens map[string]string) error {
	tx, err := b.rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}

	contactURN, err := models.GetOrCreateContactURN(ctx, tx, channel, contact.ID_, urn, authTokens)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error adding URN to contact: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	contact.URNID_ = contactURN.ID
	return nil
}
