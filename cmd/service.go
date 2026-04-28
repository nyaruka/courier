package cmd

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/backends/rapidpro"
	"github.com/nyaruka/courier/runtime"
	slogmulti "github.com/samber/slog-multi"
	slogsentry "github.com/samber/slog-sentry/v2"
)

// Service starts the courier service, blocks until a termination signal is received, then stops it.
func Service(version, date string) error {
	cfg := runtime.LoadConfig()
	cfg.Version = version

	// configure our logger
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	slog.SetDefault(slog.New(logHandler))

	// if we have a DSN entry, try to initialize it
	if cfg.SentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{Dsn: cfg.SentryDSN, ServerName: cfg.InstanceID, Release: version, AttachStacktrace: true})
		if err != nil {
			return err
		}

		defer sentry.Flush(2 * time.Second)

		slog.SetDefault(slog.New(
			slogmulti.Fanout(
				logHandler,
				slogsentry.Option{Level: slog.LevelError}.NewSentryHandler(),
			),
		))
	}

	log := slog.With("comp", "main")
	log.Info("starting courier", "version", version, "released", date)

	rt, err := runtime.NewRuntime(cfg)
	if err != nil {
		return err
	}

	backend := rapidpro.NewBackend(rt)

	server := courier.NewServer(cfg, backend)
	if err := server.Start(); err != nil {
		return err
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Info("stopping", "signal", <-ch)

	return server.Stop()
}
