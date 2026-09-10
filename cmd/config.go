package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/ezconf"
)

// LoadConfig loads configuration from a config file, environment variables and command line args, on top of the
// given base config, e.g. runtime.NewDefaultConfig(). An app built on top of courier with settings of its own can
// instead pass a struct which embeds runtime.Config, whose own fields are then loaded from the same sources and
// validated alongside the embedded ones, e.g.
//
//	type Config struct {
//		runtime.Config
//		SentryDSN string `validate:"omitempty,url" help:"the Sentry DSN to report errors to"`
//	}
//
// If the config can't be loaded, the error is logged and the process exits. If usage was requested with -help, it's
// shown and the process exits cleanly.
func LoadConfig(cfg interface{ Parse() error }) {
	Run(loadConfig(cfg, os.Args[1:]))
}

// loadConfig is LoadConfig with the args passed explicitly and the outcome returned, so that it can be tested
func loadConfig(cfg interface{ Parse() error }, args []string) error {
	loader := ezconf.NewLoader(cfg, "courier", "Courier - A fast message broker for SMS and IP messages", []string{"config.toml"})
	loader.SetArgs(args...)

	if err := loader.Load(); err != nil {
		// Load never writes to stdout or stderr itself, so a request for usage comes back as ErrHelp for us to
		// act on here, where we still have the loader to show it with. The sentinel is passed up so that the
		// caller can tell an explicit -help from a genuine config failure.
		if errors.Is(err, ezconf.ErrHelp) {
			loader.Usage()
		}
		return err
	}

	// validate the struct as a whole rather than leaving that to Parse, which only sees the embedded config when
	// given a struct that embeds it
	if err := utils.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return cfg.Parse()
}
