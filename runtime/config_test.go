package runtime_test

import (
	"net"
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
	{config: &runtime.Config{DB: "postgres://courier:courier@postgres:5432/courier", Valkey: "redis://valkey:6379/15"}, expectedError: "Field validation for 'Valkey' failed on the 'startswith=valkey:|startswith=valkeys:' tag"},
	{config: &runtime.Config{DB: "postgres://temba:temba@postgres/temba?sslmode=disable", Valkey: "valkey://valkey:6379/15", SendProxyURL: "not-a-url"}, expectedError: "Field validation for 'SendProxyURL' failed on the 'http_url' tag"},
}

func TestConfigParse(t *testing.T) {
	for _, tc := range invalidConfigTestCases {
		err := tc.config.Parse()
		if assert.Error(t, err, "expected error for config %v", tc.config) {
			assert.Contains(t, err.Error(), tc.expectedError, "error mismatch for config %v", tc.config)
		}
	}

	// parsing a valid config fills in the values which can't be used in their configured form
	cfg := runtime.NewDefaultConfig()
	cfg.SendProxyURL = "http://proxy.example.com:3128"
	require.NoError(t, cfg.Parse())

	require.NotNil(t, cfg.SendProxyURLParsed)
	assert.Equal(t, "proxy.example.com:3128", cfg.SendProxyURLParsed.Host)
	assert.Equal(t, "http", cfg.SendProxyURLParsed.Scheme)

	// the disallowed networks are split into bare IPs and CIDR networks
	assert.Equal(t, []net.IP{net.ParseIP("::1")}, cfg.DisallowedIPs)
	assert.Len(t, cfg.DisallowedNets, 9)

	// parsing again with the proxy removed clears the parsed URL rather than leaving the previous one behind
	cfg.SendProxyURL = ""
	require.NoError(t, cfg.Parse())
	assert.Nil(t, cfg.SendProxyURLParsed)

	// with no proxy configured the parsed URL stays nil, which is how newHTTP knows not to build a proxied client
	cfg = runtime.NewDefaultConfig()
	require.NoError(t, cfg.Parse())
	assert.Nil(t, cfg.SendProxyURLParsed)

	// valkeys:// is accepted as well as valkey://, so that a TLS connection can be configured
	cfg = runtime.NewDefaultConfig()
	cfg.Valkey = "valkeys://valkey:6379/15"
	assert.NoError(t, cfg.Parse())
}
