package cmd

import (
	"flag"
	"log/slog"
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/ezconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// caller can customize the base config..
	cfg := runtime.NewDefaultConfig()
	cfg.Domain = "example.com"
	cfg.DisallowedNetworks = append(cfg.DisallowedNetworks, `192.0.2.0/24`)
	cfg.LogLevel = slog.LevelError

	require.NoError(t, loadConfig(cfg, []string{`--log-level=warn`}))
	assert.Equal(t, "example.com", cfg.Domain)
	assert.Contains(t, cfg.DisallowedNetworks, `192.0.2.0/24`)
	assert.Equal(t, slog.LevelWarn, cfg.LogLevel)

	// but explicitly set values still take precedence
	cfg = runtime.NewDefaultConfig()
	cfg.Domain = "example.com"
	cfg.DisallowedNetworks = append(cfg.DisallowedNetworks, `192.0.2.0/24`)

	require.NoError(t, loadConfig(cfg, []string{`--domain=temba.io`}))
	assert.Equal(t, "temba.io", cfg.Domain)
	assert.Contains(t, cfg.DisallowedNetworks, `192.0.2.0/24`)

	// and the loaded config is parsed so it's ready to be used
	assert.Len(t, cfg.DisallowedNets, 10)

	// invalid values are rejected
	err := loadConfig(runtime.NewDefaultConfig(), []string{`--disallowed-networks="127.0.0.1`})
	assert.Error(t, err)

	// as are values which fail validation
	err = loadConfig(runtime.NewDefaultConfig(), []string{`--db=mysql:test`})
	assert.ErrorContains(t, err, "Field validation for 'DB' failed on the 'startswith' tag")

	// asking for usage comes back as the ErrHelp sentinel rather than exiting the process, so that Run can exit
	// cleanly instead of reporting a config failure
	err = loadConfig(runtime.NewDefaultConfig(), []string{`--help`})
	assert.ErrorIs(t, err, ezconf.ErrHelp)
	assert.ErrorIs(t, err, flag.ErrHelp)
}

// wrappedConfig is how an app built on top of courier adds its own settings to the config
type wrappedConfig struct {
	runtime.Config

	APIKey string `validate:"required" help:"the key used to access the wrapping app's API"`
}

func TestLoadConfigEmbedded(t *testing.T) {
	t.Setenv("COURIER_API_KEY", "sesame")

	// the embedded config's fields and the wrapping struct's own fields are loaded from the same sources
	cfg := &wrappedConfig{Config: *runtime.NewDefaultConfig()}
	require.NoError(t, loadConfig(cfg, []string{`--domain=temba.io`}))
	assert.Equal(t, "temba.io", cfg.Domain)
	assert.Equal(t, "sesame", cfg.APIKey)

	// and the embedded config is parsed so it's ready to be handed to the service
	assert.Len(t, cfg.DisallowedNets, 9)

	// validation covers the wrapping struct's own fields...
	t.Setenv("COURIER_API_KEY", "")
	cfg = &wrappedConfig{Config: *runtime.NewDefaultConfig()}
	err := loadConfig(cfg, []string{`--domain=temba.io`})
	assert.ErrorContains(t, err, "Field validation for 'APIKey' failed on the 'required' tag")

	// ...as well as the embedded ones
	t.Setenv("COURIER_API_KEY", "sesame")
	cfg = &wrappedConfig{Config: *runtime.NewDefaultConfig()}
	err = loadConfig(cfg, []string{`--db=mysql:test`})
	assert.ErrorContains(t, err, "Field validation for 'DB' failed on the 'startswith' tag")

	// and asking for usage still comes back as the ErrHelp sentinel
	cfg = &wrappedConfig{Config: *runtime.NewDefaultConfig()}
	err = loadConfig(cfg, []string{`--help`})
	assert.ErrorIs(t, err, ezconf.ErrHelp)
}
