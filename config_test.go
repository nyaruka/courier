package courier_test

import (
	"testing"

	"github.com/nyaruka/courier"
	"github.com/stretchr/testify/assert"
)

var invalidConfigTestCases = []struct {
	config        *courier.Config
	expectedError string
}{
	{config: &courier.Config{DB: ":foo", Valkey: "valkey:localhost/23"}, expectedError: "Field validation for 'DB' failed on the 'url' tag"},
	{config: &courier.Config{DB: "mysql:test", Valkey: "valkey:localhost/23"}, expectedError: "Field validation for 'DB' failed on the 'startswith' tag"},
	{config: &courier.Config{DB: "postgres://courier:courier@localhost:5432/courier", Valkey: ":foo"}, expectedError: "Field validation for 'Valkey' failed on the 'url' tag"},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range invalidConfigTestCases {
		err := tc.config.Validate()
		if assert.Error(t, err, "expected error for config %v", tc.config) {
			assert.Contains(t, err.Error(), tc.expectedError, "error mismatch for config %v", tc.config)
		}
	}
}
