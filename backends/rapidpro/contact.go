package rapidpro

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
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
