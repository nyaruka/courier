package courier

import "testing"

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
		urn := NewTelURN(tc.number, tc.country)
		if urn != URN(tc.expected) {
			t.Errorf("Failed tel parsing, got '%s', expected '%s' for '%s:%s'", urn, tc.expected, tc.number, tc.country)
		}
	}
}

func TestTelegramURNs(t *testing.T) {
	testCases := []struct {
		identifier int64
		expected   string
	}{
		{12345, "telegram:12345"},
	}

	for _, tc := range testCases {
		urn := NewTelegramURN(tc.identifier)
		if urn != URN(tc.expected) {
			t.Errorf("Failed telegram URN, got '%s', expected '%s' for '%d'", urn, tc.expected, tc.identifier)
		}
	}
}

func TestFromParts(t *testing.T) {
	testCases := []struct {
		scheme   string
		path     string
		expected string
		err      bool
	}{
		{"TEL", "+250788383383", "tel:+250788383383", false},
		{"telephone", "+250788383383", "", true},
		{"twitter", "hello", "twitter:hello", false},
		{"facebook", "hello", "facebook:hello", false},
		{"telegram", "12345", "telegram:12345", false},
	}

	for _, tc := range testCases {
		urn, err := NewURNFromParts(tc.scheme, tc.path)
		if err != nil && !tc.err {
			t.Errorf("Unexpected error creating urn: %s:%s: %s", tc.scheme, tc.path, err)
		}
		if err == nil && tc.err {
			t.Errorf("Expected error creating urn: %s:%s: ", tc.scheme, tc.path)
		}

		if urn != URN(tc.expected) {
			t.Errorf("Failed creating urn, got '%s', expected '%s' for '%s:%s'", urn, tc.expected, tc.path, tc.scheme)
		}
	}
}

func BenchmarkValidTel(b *testing.B) {
	for n := 0; n < b.N; n++ {
		NewTelURN("2065551212", "US")
	}
}

func BenchmarkInvalidTel(b *testing.B) {
	for n := 0; n < b.N; n++ {
		NewTelURN("notnumber", "US")
	}
}
