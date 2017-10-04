package urns

import (
	"strconv"
	"testing"
)

func TestFromParts(t *testing.T) {
	testCases := []struct {
		scheme   string
		path     string
		display  string
		expected string
		identity string
	}{
		{"tel", "+250788383383", "", "tel:+250788383383", "tel:+250788383383"},
		{"twitter", "hello", "", "twitter:hello", "twitter:hello"},
		{"facebook", "hello", "", "facebook:hello", "facebook:hello"},
		{"telegram", "12345", "Jane", "telegram:12345#jane", "telegram:12345"},
	}

	for _, tc := range testCases {
		urn := NewURNFromParts(tc.scheme, tc.path, tc.display)
		if urn != URN(tc.expected) {
			t.Errorf("Failed creating urn, got '%s', expected '%s' for '%s:%s'", urn, tc.expected, tc.scheme, tc.path)
		}
		if urn.Identity() != tc.identity {
			t.Errorf("Failed creating urn, got identity '%s', expected identity '%s' for '%s:%s'", urn, tc.expected, tc.scheme, tc.path)
		}
	}
}

func TestNormalize(t *testing.T) {
	testCases := []struct {
		rawURN   URN
		country  string
		expected URN
	}{
		// valid tel numbers
		{"tel:0788383383", "RW", "tel:+250788383383"},
		{"tel: +250788383383 ", "KE", "tel:+250788383383"},
		{"tel:+250788383383", "", "tel:+250788383383"},
		{"tel:250788383383", "", "tel:+250788383383"},
		{"tel:2.50788383383E+11", "", "tel:+250788383383"},
		{"tel:2.50788383383E+12", "", "tel:+250788383383"},
		{"tel:(917)992-5253", "US", "tel:+19179925253"},
		{"tel:19179925253", "", "tel:+19179925253"},
		{"tel:+62877747666", "", "tel:+62877747666"},
		{"tel:62877747666", "ID", "tel:+62877747666"},
		{"tel:0877747666", "ID", "tel:+62877747666"},
		{"tel:07531669965", "GB", "tel:+447531669965"},

		// un-normalizable tel numbers
		{"tel:12345", "RW", "tel:12345"},
		{"tel:0788383383", "", "tel:0788383383"},
		{"tel:0788383383", "ZZ", "tel:0788383383"},
		{"tel:MTN", "RW", "tel:mtn"},

		// twitter handles remove @
		{"twitter: @jimmyJO", "", "twitter:jimmyjo"},
		{"twitterid:12345#@jimmyJO", "", "twitterid:12345#jimmyjo"},

		// email addresses
		{"mailto: nAme@domAIN.cOm ", "", "mailto:name@domain.com"},

		// external ids are case sensitive
		{"ext: eXterNAL123 ", "", "ext:eXterNAL123"},
	}

	for _, tc := range testCases {
		normalized := tc.rawURN.Normalize(tc.country)
		if normalized != tc.expected {
			t.Errorf("Failed normalizing urn, got '%s', expected '%s' for '%s' in country %s", normalized, tc.expected, string(tc.rawURN), tc.country)
		}
	}
}

func TestLocalize(t *testing.T) {
	testCases := []struct {
		input    URN
		country  string
		expected URN
	}{
		// valid tel numbers
		{"tel:+250788383383", "RW", "tel:788383383"},
		{"tel:+447531669965", "GB", "tel:7531669965"},
		{"tel:+19179925253", "US", "tel:9179925253"},

		// un-localizable tel numbers
		{"tel:12345", "RW", "tel:12345"},
		{"tel:0788383383", "", "tel:0788383383"},
		{"tel:0788383383", "ZZ", "tel:0788383383"},
		{"tel:MTN", "RW", "tel:MTN"},

		// other schemes left as is
		{"twitter:jimmyjo", "RW", "twitter:jimmyjo"},
		{"mailto:bob@example.com", "", "mailto:bob@example.com"},
	}

	for _, tc := range testCases {
		localized := tc.input.Localize(tc.country)
		if localized != tc.expected {
			t.Errorf("Failed localizing urn, got '%s', expected '%s' for '%s' in country %s", localized, tc.expected, string(tc.input), tc.country)
		}
	}
}

func TestValidate(t *testing.T) {
	testCases := []struct {
		urn     URN
		isValid bool
	}{
		{"xxxx", false},    // un-parseable URNs don't validate
		{"xyz:abc", false}, // nor do unknown schemes

		// valid tel numbers
		{"tel:+250788383383", true},
		{"tel:+23761234567", true},  // old Cameroon format
		{"tel:+237661234567", true}, // new Cameroon format
		{"tel:+250788383383", true},

		// invalid tel numbers
		{"tel:0788383383", false}, // no country
		{"tel:MTN", false},

		// twitter handles
		{"twitter:jimmyjo", true},
		{"twitter:billy_bob", true},
		{"twitter:jimmyjo!@", false},
		{"twitter:billy bob", false},

		// twitterid urns
		{"twitterid:12345#jimmyjo", true},
		{"twitterid:12345#1234567", true},
		{"twitterid:jimmyjo#1234567", false},
		{"twitterid:123#a.!f", false},

		// email addresses
		{"mailto:abcd+label@x.y.z.com", true},
		{"mailto:@@@", false},

		// facebook and telegram URN paths must be integers
		{"telegram:12345678901234567", true},
		{"telegram:abcdef", false},
		{"facebook:12345678901234567", true},
		{"facebook:abcdef", false},
	}

	for _, tc := range testCases {
		isValid := tc.urn.Validate()
		if isValid != tc.isValid {
			t.Errorf("Failed validating urn, got %s, expected %s for '%s'", strconv.FormatBool(isValid), strconv.FormatBool(tc.isValid), string(tc.urn))
		}
	}
}

func TestTelURNs(t *testing.T) {
	testCases := []struct {
		number   string
		country  string
		expected string
	}{
		{"0788383383", "RW", "tel:+250788383383"},
		{" +250788383383 ", "KE", "tel:+250788383383"},
		{"+250788383383", "", "tel:+250788383383"},
		{"250788383383", "", "tel:+250788383383"},
		{"(917)992-5253", "US", "tel:+19179925253"},
		{"19179925253", "", "tel:+19179925253"},
		{"+62877747666", "", "tel:+62877747666"},
		{"62877747666", "ID", "tel:+62877747666"},
		{"0877747666", "ID", "tel:+62877747666"},
		{"07531669965", "GB", "tel:+447531669965"},
		{"12345", "RW", "tel:12345"},
		{"0788383383", "", "tel:0788383383"},
		{"0788383383", "ZZ", "tel:0788383383"},
		{"MTN", "RW", "tel:mtn"},
	}

	for _, tc := range testCases {
		urn := NewTelURNForCountry(tc.number, tc.country)
		if urn != URN(tc.expected) {
			t.Errorf("Failed tel parsing, got '%s', expected '%s' for '%s:%s'", urn, tc.expected, tc.number, tc.country)
		}
	}
}

func TestTelegramURNs(t *testing.T) {
	testCases := []struct {
		identifier int64
		display    string
		expected   string
	}{
		{12345, "", "telegram:12345"},
		{12345, "Sarah", "telegram:12345#sarah"},
	}

	for _, tc := range testCases {
		urn := NewTelegramURN(tc.identifier, tc.display)
		if urn != URN(tc.expected) {
			t.Errorf("Failed telegram URN, got '%s', expected '%s' for '%d'", urn, tc.expected, tc.identifier)
		}
	}
}

func BenchmarkValidTel(b *testing.B) {
	for n := 0; n < b.N; n++ {
		NewTelURNForCountry("2065551212", "US")
	}
}

func BenchmarkInvalidTel(b *testing.B) {
	for n := 0; n < b.N; n++ {
		NewTelURNForCountry("notnumber", "US")
	}
}
