package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/core/sender"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/courier/v26/web"
)

// shutdownTimeout is how long we allow for a graceful shutdown before exiting hard. The shutdown phases are
// individually bounded (in-flight requests and the final status flush at 30s, in-flight sends at 35s) but run
// partly in sequence, so a legitimate slow shutdown can approach ~60s. Past this budget something is genuinely
// wedged, and it's better to exit with a record of why than be killed by the orchestrator. Orchestrator stop
// timeouts should be set a bit above this so the watchdog fires first.
const shutdownTimeout = 90 * time.Second

// Service starts the courier service, blocks until a termination signal is received, then stops it. Configuration
// is loaded on top of the given defaults, e.g. runtime.NewDefaultConfig(). All logging is sent to the given handler,
// e.g. LogHandler(), whose level is set from the loaded config.
func Service(defaults *runtime.Config, version, date string, logHandler slog.Handler) error {
	cfg, err := runtime.LoadConfig(defaults)
	if err != nil {
		return err
	}
	cfg.Version = version

	// configure our logger
	logLevel.Set(cfg.LogLevel)
	slog.SetDefault(slog.New(logHandler))

	log := slog.With("comp", "main")
	log.Info("starting courier", "version", version, "released", date)

	rt, err := runtime.NewRuntime(cfg)
	if err != nil {
		return err
	}

	// log what we can and can't reach before we start doing anything with it
	testConnections(rt)

	svc, err := startService(rt)
	if err != nil {
		return err
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Info("stopping", "signal", <-ch)

	watchdog := time.AfterFunc(shutdownTimeout, func() {
		log.Error("shutdown timed out, exiting", "timeout", shutdownTimeout)
		os.Exit(1)
	})
	defer watchdog.Stop()

	svc.stop()

	return nil
}

// service is the set of components this process runs, in the order they're started
type service struct {
	rt      *runtime.Runtime
	quit    chan bool
	waitFor *sync.WaitGroup
	server  *web.Server
	foreman *sender.Foreman
}

// startService starts each component in turn, unwinding whatever is already running if one of them fails, so that a
// failure part way through doesn't leave the process with half a runtime
func startService(rt *runtime.Runtime) (*service, error) {
	s := &service{rt: rt, quit: make(chan bool), waitFor: &sync.WaitGroup{}}

	if err := rt.Start(); err != nil {
		return nil, fmt.Errorf("error starting runtime: %w", err)
	}

	// start the caches, spools and batched writers used by the read and write paths
	if err := models.Start(rt); err != nil {
		rt.Stop()
		return nil, err
	}

	// start our dethrottler if we are going to be doing some sending
	if rt.Config.MaxWorkers > 0 {
		queue.StartDethrottler(rt.VK, s.quit, s.waitFor, models.MsgQueueName)
	}

	startMetricsReporter(rt, time.Minute, s.quit, s.waitFor)

	server := web.NewServer(rt)
	if err := server.Start(); err != nil {
		s.stop()
		return nil, err
	}
	s.server = server

	foreman := sender.NewForeman(rt, rt.Config.MaxWorkers)
	foreman.Start()
	s.foreman = foreman

	return s, nil
}

// stop stops each component in the reverse of the order it was started, skipping those which never started
func (s *service) stop() {
	// stop sending first so that in-flight sends finish before the writers they depend on go away
	if s.foreman != nil {
		s.foreman.Stop()
	}
	if s.server != nil {
		if err := s.server.Stop(); err != nil {
			slog.Error("error stopping server", "error", err)
		}
	}

	// stop the dethrottler and metrics reporter
	close(s.quit)
	s.waitFor.Wait()

	// then the caches, spools and batched writers, and finally the runtime itself
	models.Stop()
	s.rt.Stop()
}
