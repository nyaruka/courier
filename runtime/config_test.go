package runtime_test

import (
	"log/slog"
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

func TestLoadConfig(t *testing.T) {
	// caller can customize the base config..
	base := runtime.NewDefaultConfig()
	base.Domain = "example.com"
	base.DisallowedNetworks = append(base.DisallowedNetworks, `192.0.2.0/24`)
	base.LogLevel = slog.LevelError

	cfg, err := runtime.LoadConfig(base, `--log-level=warn`)
	assert.NoError(t, err)
	assert.Equal(t, "example.com", cfg.Domain)
	assert.Contains(t, cfg.DisallowedNetworks, `192.0.2.0/24`)
	assert.Equal(t, slog.LevelWarn, cfg.LogLevel)

	// but explicitly set values still take precedence
	base = runtime.NewDefaultConfig()
	base.Domain = "example.com"
	base.DisallowedNetworks = append(base.DisallowedNetworks, `192.0.2.0/24`)

	cfg, err = runtime.LoadConfig(base, `--domain=temba.io`)
	assert.NoError(t, err)
	assert.Equal(t, "temba.io", cfg.Domain)
	assert.Contains(t, cfg.DisallowedNetworks, `192.0.2.0/24`)

	// invalid values are rejected
	_, err = runtime.LoadConfig(runtime.NewDefaultConfig(), `--disallowed-networks="127.0.0.1`)
	assert.Error(t, err)
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range invalidConfigTestCases {
		err := tc.config.Validate()
		if assert.Error(t, err, "expected error for config %v", tc.config) {
			assert.Contains(t, err.Error(), tc.expectedError, "error mismatch for config %v", tc.config)
		}
	}
}

func TestParseSendProxyURL(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.SendProxyURL = "http://proxy.example.com:3128"
	require.NoError(t, cfg.Validate())

	u, err := cfg.ParseSendProxyURL()
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "proxy.example.com:3128", u.Host)
	assert.Equal(t, "http", u.Scheme)

	// empty SendProxyURL returns (nil, nil)
	cfg = runtime.NewDefaultConfig()
	require.NoError(t, cfg.Validate())
	u, err = cfg.ParseSendProxyURL()
	require.NoError(t, err)
	assert.Nil(t, u)
}
