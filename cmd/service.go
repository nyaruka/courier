package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/core/sender"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/courier/v26/web"
	slogmulti "github.com/samber/slog-multi"
	slogsentry "github.com/samber/slog-sentry/v2"
)

// shutdownTimeout is how long we allow for a graceful shutdown before exiting hard. The shutdown phases are
// individually bounded (in-flight requests and the final status flush at 30s, in-flight sends at 35s) but run
// partly in sequence, so a legitimate slow shutdown can approach ~60s. Past this budget something is genuinely
// wedged, and it's better to exit with a record of why than be killed by the orchestrator. Orchestrator stop
// timeouts should be set a bit above this so the watchdog fires first.
const shutdownTimeout = 90 * time.Second

// Service starts the courier service, blocks until a termination signal is received, then stops it. Configuration
// is loaded on top of the given defaults, e.g. runtime.NewDefaultConfig().
func Service(defaults *runtime.Config, version, date string) error {
	cfg, err := runtime.LoadConfig(defaults)
	if err != nil {
		return err
	}
	cfg.Version = version

	// configure our logger
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	slog.SetDefault(slog.New(logHandler))

	// if we have a DSN entry, try to initialize it
	if cfg.SentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{Dsn: cfg.SentryDSN, Release: version, AttachStacktrace: true})
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

	// log what we can and can't reach before we start doing anything with it
	testConnections(rt)

	if err := rt.Start(); err != nil {
		return fmt.Errorf("error starting runtime: %w", err)
	}

	// start the caches, spools and batched writers used by the read and write paths
	if err := models.Start(rt); err != nil {
		return err
	}

	stop := make(chan bool)
	wg := &sync.WaitGroup{}

	// start our dethrottler if we are going to be doing some sending
	if cfg.MaxWorkers > 0 {
		queue.StartDethrottler(rt.VK, stop, wg, models.MsgQueueName)
	}

	startMetricsReporter(rt, time.Minute, stop, wg)

	server := web.NewServer(rt)
	if err := server.Start(); err != nil {
		return err
	}

	foreman := sender.NewForeman(rt, cfg.MaxWorkers)
	foreman.Start()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Info("stopping", "signal", <-ch)

	watchdog := time.AfterFunc(shutdownTimeout, func() {
		log.Error("shutdown timed out, exiting", "timeout", shutdownTimeout)
		sentry.Flush(2 * time.Second)
		os.Exit(1)
	})
	defer watchdog.Stop()

	// stop sending first so in-flight sends finish before the writers they depend on go away
	foreman.Stop()

	if err := server.Stop(); err != nil {
		return err
	}

	// stop the dethrottler and metrics reporter
	close(stop)
	wg.Wait()

	// stop the caches, spools and batched writers, then the runtime itself
	models.Stop()
	rt.Stop()

	return nil
}
