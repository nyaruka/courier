package runtime_test

import (
	"testing"

	"github.com/nyaruka/courier/runtime"
	"github.com/stretchr/testify/assert"
)

var invalidConfigTestCases = []struct {
	config        *runtime.Config
	expectedError string
}{
	{config: &runtime.Config{DB: ":foo", Redis: "redis:localhost/23"}, expectedError: "Field validation for 'DB' failed on the 'url' tag"},
	{config: &runtime.Config{DB: "mysql:test", Redis: "redis:localhost/23"}, expectedError: "Field validation for 'DB' failed on the 'startswith' tag"},
	{config: &runtime.Config{DB: "postgres://courier:courier@localhost:5432/courier", Redis: ":foo"}, expectedError: "Field validation for 'Redis' failed on the 'url' tag"},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range invalidConfigTestCases {
		err := tc.config.Validate()
		if assert.Error(t, err, "expected error for config %v", tc.config) {
			assert.Contains(t, err.Error(), tc.expectedError, "error mismatch for config %v", tc.config)
		}
	}
}
