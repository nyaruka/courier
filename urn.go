package courier

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// URN represents a Universal Resource Name, we use this for contact identifiers like phone numbers etc..
type URN string

// NilURN is our nil value for URN
var NilURN = URN("")

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

var validSchemes = map[string]bool{
	FacebookScheme: true,
	TelegramScheme: true,
	TelScheme:      true,
	TwitterScheme:  true,
}

var telRegex = regexp.MustCompile(`[^0-9a-z]`)

// NewTelegramURN returns a URN for the passed in telegram identifier
func NewTelegramURN(identifier int64) URN {
	return newURN(TelegramScheme, fmt.Sprintf("%d", identifier))
}

// NewTelURN returns a URN for the passed in telephone number and country code ("US")
func NewTelURN(number string, country string) URN {
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
