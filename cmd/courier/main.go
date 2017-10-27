package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/evalphobia/logrus_sentry"
	_ "github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/sirupsen/logrus"

	// load channel handler packages
	_ "github.com/nyaruka/courier/handlers/africastalking"
	_ "github.com/nyaruka/courier/handlers/blackmyna"
	_ "github.com/nyaruka/courier/handlers/dmark"
	_ "github.com/nyaruka/courier/handlers/kannel"
	_ "github.com/nyaruka/courier/handlers/shaqodoon"
	_ "github.com/nyaruka/courier/handlers/telegram"
	_ "github.com/nyaruka/courier/handlers/twilio"

	// load available backends
	_ "github.com/nyaruka/courier/backends/rapidpro"
)

var version = "Dev"

func main() {
	m := config.NewWithPath("courier.toml")
	config := &config.Courier{}
	err := m.Load(config)
	if err != nil {
		logrus.Fatalf("Error loading configuration: %s", err)
	}

	// if we have a custom version, use it
	if version != "Dev" {
		config.Version = version
	}

	// configure our logger
	logrus.SetOutput(os.Stdout)
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		logrus.Fatalf("Invalid log level '%s'", level)
	}
	logrus.SetLevel(level)

	// if we have a DSN entry, try to initialize it
	if config.SentryDSN != "" {
		hook, err := logrus_sentry.NewSentryHook(config.SentryDSN, []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel})
		hook.Timeout = 0
		hook.StacktraceConfiguration.Enable = true
		hook.StacktraceConfiguration.Skip = 4
		hook.StacktraceConfiguration.Context = 5
		if err != nil {
			logrus.Fatalf("Invalid sentry DSN: '%s': %s", config.SentryDSN, err)
		}
		logrus.StandardLogger().Hooks.Add(hook)
	}

	// load our backend
	backend, err := courier.NewBackend(config)
	if err != nil {
		logrus.Fatalf("Error creating backend: %s", err)
	}

	server := courier.NewServer(config, backend)
	err = server.Start()
	if err != nil {
		logrus.Fatalf("Error starting server: %s", err)
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	logrus.WithField("comp", "main").WithField("signal", <-ch).Info("stopping")

	server.Stop()
}
