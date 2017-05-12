package courier

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/phonenumbers"
)

const (
	// FacebookScheme is the scheme used for Facebook identifiers
	FacebookScheme string = "facebook"

	// TelegramScheme is the scheme used for telegram identifier
	TelegramScheme string = "telegram"

	// TelScheme is the scheme used for telephone numbers
	TelScheme string = "tel"

	// TwitterScheme is the scheme used for Twitter identifiers
	TwitterScheme string = "twitter"
)

// ContactURNID represents a contact urn's id
type ContactURNID struct {
	sql.NullInt64
}

// NilContactURNID is our nil value for ContactURNID
var NilContactURNID = ContactURNID{sql.NullInt64{Int64: 0, Valid: false}}

// ContactURN is our struct to map to database level URNs
type ContactURN struct {
	Org      OrgID        `db:"org_id"`
	ID       ContactURNID `db:"id"`
	URN      URN          `db:"urn"`
	Scheme   string       `db:"scheme"`
	Path     string       `db:"path"`
	Priority int          `db:"priority"`
	Channel  ChannelID    `db:"channel_id"`
	Contact  ContactID    `db:"contact_id"`
}

// URN represents a Universal Resource Name, we use this for contact identifiers like phone numbers etc..
type URN string

// NilURN is our constant for nil URNs
var NilURN = URN("")

// NewTelegramURN returns a URN for the passed in telegram identifier
func NewTelegramURN(identifier int64) URN {
	return newURN(TelegramScheme, fmt.Sprintf("%d", identifier))
}

// NewTelURNForChannel returns a URN for the passed in telephone number and channel
func NewTelURNForChannel(number string, channel *Channel) URN {
	return NewTelURNForCountry(number, channel.Country.String)
}

// NewTelURNForCountry returns a URN for the passed in telephone number and country code ("US")
func NewTelURNForCountry(number string, country string) URN {
	// add on a plus if it looks like it could be a fully qualified number
	number = telRegex.ReplaceAllString(strings.ToLower(strings.TrimSpace(number)), "")
	parseNumber := number
	if len(number) >= 11 && !(strings.HasPrefix(number, "+") || strings.HasPrefix(number, "0")) {
		parseNumber = fmt.Sprintf("+%s", number)
	}

	normalized, err := phonenumbers.Parse(parseNumber, country)

	// couldn't parse it, use the original number
	if err != nil {
		return newURN(TelScheme, number)
	}

	// if it looks valid, return it
	if phonenumbers.IsValidNumber(normalized) {
		return newURN(TelScheme, phonenumbers.Format(normalized, phonenumbers.E164))
	}

	// this doesn't look like anything we recognize, use the original number
	return newURN(TelScheme, number)
}

// NewURNFromParts returns a new URN for the given scheme and path
func NewURNFromParts(scheme string, path string) (URN, error) {
	scheme = strings.ToLower(scheme)
	if !validSchemes[scheme] {
		return NilURN, fmt.Errorf("invalid scheme '%s'", scheme)
	}

	return newURN(scheme, path), nil
}

// private utility method to create a URN from a scheme and path
func newURN(scheme string, path string) URN {
	return URN(fmt.Sprintf("%s:%s", scheme, path))
}

// NewContactURN returns a new ContactURN object for the passed in org, contact and string urn, this is not saved to the DB yet
func NewContactURN(org OrgID, channel ChannelID, contact ContactID, urn URN) *ContactURN {
	offset := strings.Index(string(urn), ":")
	scheme := string(urn)[:offset]
	path := string(urn)[offset+1:]

	return &ContactURN{Org: org, Channel: channel, Contact: contact, URN: urn, Scheme: scheme, Path: path}
}

const selectOrgURN = `
SELECT org_id, id, urn, scheme, path, priority, channel_id, contact_id 
FROM contacts_contacturn
WHERE org_id = $1 AND urn = $2
ORDER BY priority desc LIMIT 1
`

// ContactURNForURN returns the ContactURN for the passed in org and URN, creating and associating
// it with the passed in contact if necessary
func ContactURNForURN(db *sqlx.DB, org OrgID, channel ChannelID, contact ContactID, urn URN) (*ContactURN, error) {
	contactURN := NewContactURN(org, channel, contact, urn)
	err := db.Get(contactURN, selectOrgURN, org, urn)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// we didn't find it, let's insert it
	if err == sql.ErrNoRows {
		err = InsertContactURN(db, contactURN)
		if err != nil {
			return nil, err
		}
	}

	// make sure our contact URN is up to date
	if contactURN.Channel != channel || contactURN.Contact != contact {
		contactURN.Channel = channel
		contactURN.Contact = contact
		err = UpdateContactURN(db, contactURN)
	}

	return contactURN, err
}

const insertURN = `
INSERT INTO contacts_contacturn(org_id, urn, path, scheme, priority, channel_id, contact_id)
VALUES(:org_id, :urn, :path, :scheme, :priority, :channel_id, :contact_id)
RETURNING id
`

// InsertContactURN inserts the passed in urn, the id field will be populated with the result on success
func InsertContactURN(db *sqlx.DB, urn *ContactURN) error {
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
func UpdateContactURN(db *sqlx.DB, urn *ContactURN) error {
	rows, err := db.NamedQuery(updateURN, urn)
	if err != nil {
		return err
	}
	if rows.Next() {
		rows.Scan(&urn.ID)
	}
	return err
}

var validSchemes = map[string]bool{
	FacebookScheme: true,
	TelegramScheme: true,
	TelScheme:      true,
	TwitterScheme:  true,
}

var telRegex = regexp.MustCompile(`[^0-9a-z]`)
