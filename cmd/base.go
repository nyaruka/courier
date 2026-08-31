package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/nyaruka/ezconf"
)

// the level of the standard log handler, set from the config when the service starts
var logLevel = &slog.LevelVar{}

// LogHandler returns the standard log handler, which logs at the level configured by COURIER_LOG_LEVEL.
func LogHandler() slog.Handler {
	return slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
}

// Run logs the given error and exits with a non-zero status code if err is not nil. Asking for usage with -help
// isn't a failure - it's already been shown by the config loader - so that exits cleanly and without logging.
func Run(err error) {
	if err != nil {
		if errors.Is(err, ezconf.ErrHelp) {
			os.Exit(0)
		}
		slog.Error(err.Error())
		os.Exit(1)
	}
}
