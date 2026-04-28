package cmd

import (
	"log/slog"
	"os"
)

// Run logs the given error and exits with a non-zero status code if err is not nil.
func Run(err error) {
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
