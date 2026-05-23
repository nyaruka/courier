package runtime_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var invalidConfigTestCases = []struct {
	config        *runtime.Config
	expectedError string
}{
	{config: &runtime.Config{DB: ":foo", Valkey: "valkey:valkey/23"}, expectedError: "Field validation for 'DB' failed on the 'url' tag"},
	{config: &runtime.Config{DB: "mysql:test", Valkey: "valkey:valkey/23"}, expectedError: "Field validation for 'DB' failed on the 'startswith' tag"},
	{config: &runtime.Config{DB: "postgres://courier:courier@postgres:5432/courier", Valkey: ":foo"}, expectedError: "Field validation for 'Valkey' failed on the 'url' tag"},
	{config: &runtime.Config{DB: "postgres://temba:temba@postgres/temba?sslmode=disable", Valkey: "valkey://valkey:6379/15", SendProxyURL: "not-a-url"}, expectedError: "Field validation for 'SendProxyURL' failed on the 'http_url' tag"},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range invalidConfigTestCases {
		err := tc.config.Validate()
		if assert.Error(t, err, "expected error for config %v", tc.config) {
			assert.Contains(t, err.Error(), tc.expectedError, "error mismatch for config %v", tc.config)
		}
	}
}

func TestSendProxyURLParsing(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.SendProxyURL = "http://proxy.example.com:3128"

	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.SendProxyURLParsed)
	assert.Equal(t, "proxy.example.com:3128", cfg.SendProxyURLParsed.Host)
	assert.Equal(t, "http", cfg.SendProxyURLParsed.Scheme)

	// empty SendProxyURL leaves SendProxyURLParsed nil
	cfg = runtime.NewDefaultConfig()
	require.NoError(t, cfg.Validate())
	assert.Nil(t, cfg.SendProxyURLParsed)
}
