package courier

import (
	"bytes"
	"compress/flate"
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/analytics"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

// for use in request.Context
type contextKey int

const (
	contextRequestURL contextKey = iota
	contextRequestStart
)

// Server is the main interface ChannelHandlers use to interact with backends. It provides an
// abstraction that makes mocking easier for isolated unit tests
type Server interface {
	Config() *Config

	AddHandlerRoute(handler ChannelHandler, method string, action string, logType ChannelLogType, handlerFunc ChannelHandleFunc)
	GetHandler(Channel) ChannelHandler

	Backend() Backend

	WaitGroup() *sync.WaitGroup
	StopChan() chan bool
	Stopped() bool

	Router() chi.Router

	Start() error
	Stop() error
}

// NewServer creates a new Server for the passed in configuration. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServer(config *Config, backend Backend) Server {
	// create our top level router
	logger := slog.Default()
	return NewServerWithLogger(config, backend, logger)
}

// NewServerWithLogger creates a new Server for the passed in configuration. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServerWithLogger(config *Config, backend Backend, logger *slog.Logger) Server {
	router := chi.NewRouter()
	router.Use(middleware.Compress(flate.DefaultCompression))
	router.Use(middleware.StripSlashes)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))

	publicRouter := chi.NewRouter()
	router.Mount("/c/", publicRouter)

	return &server{
		config:  config,
		backend: backend,

		router:       router,
		publicRouter: publicRouter,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},
		stopped:   false,
	}
}

// Start starts the Server listening for incoming requests and sending messages. It will return an error
// if it encounters any unrecoverable (or ignorable) error, though its bias is to move forward despite
// connection errors
func (s *server) Start() error {
	// configure librato if we have configuration options for it
	host, _ := os.Hostname()
	if s.config.LibratoUsername != "" {
		analytics.RegisterBackend(analytics.NewLibrato(s.config.LibratoUsername, s.config.LibratoToken, host, time.Second, s.waitGroup))
	}

	analytics.Start()

	// start our backend
	err := s.backend.Start()
	if err != nil {
		return err
	}

	// start our spool flushers
	startSpoolFlushers(s)

	// wire up our main pages
	s.router.NotFound(s.handle404)
	s.router.MethodNotAllowed(s.handle405)
	s.router.Get("/", s.handleIndex)
	s.router.Get("/status", s.basicAuthRequired(s.handleStatus))
	s.publicRouter.Post("/_fetch-attachment", s.tokenAuthRequired(s.handleFetchAttachment)) // becomes /c/_fetch-attachment

	// initialize our handlers
	s.initializeChannelHandlers()

	// configure timeouts on our server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Address, s.config.Port),
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	s.waitGroup.Add(1)

	// and start serving HTTP
	go func() {
		defer s.waitGroup.Done()
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err, "comp", "server", "state", "stopping")
		}
	}()

	s.waitGroup.Add(1)

	// start our heartbeat
	go func() {
		defer s.waitGroup.Done()

		for !s.stopped {
			select {
			case <-s.stopChan:
				return
			case <-time.After(time.Minute):
				err := s.backend.Heartbeat()
				if err != nil {
					slog.Error("error running backend heartbeat", "error", err)
				}
			}
		}
	}()

	slog.Info(fmt.Sprintf("server listening on %d", s.config.Port),
		"comp", "server",
		"port", s.config.Port,
		"state", "started",
		"version", s.config.Version,
	)

	// start our foreman for outgoing messages
	s.foreman = NewForeman(s, s.config.MaxWorkers)
	s.foreman.Start()

	return nil
}

// Stop stops the server, returning only after all threads have stopped
func (s *server) Stop() error {
	log := slog.With("comp", "server")
	log.Info("stopping server", "state", "stopping")

	// stop our foreman
	s.foreman.Stop()

	// shut down our HTTP server
	if err := s.httpServer.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down server", "error", err, "state", "stopping")
	}

	// stop everything
	s.stopped = true
	close(s.stopChan)

	// stop our backend
	err := s.backend.Stop()
	if err != nil {
		return err
	}

	analytics.Stop()

	// wait for everything to stop
	s.waitGroup.Wait()

	// clean things up, tearing down any connections
	s.backend.Cleanup()
	log.Info("server stopped", "state", "stopped")
	return nil
}

func (s *server) GetHandler(ch Channel) ChannelHandler { return activeHandlers[ch.ChannelType()] }

func (s *server) WaitGroup() *sync.WaitGroup { return s.waitGroup }
func (s *server) StopChan() chan bool        { return s.stopChan }
func (s *server) Config() *Config            { return s.config }
func (s *server) Stopped() bool              { return s.stopped }

func (s *server) Backend() Backend   { return s.backend }
func (s *server) Router() chi.Router { return s.router }

type server struct {
	backend Backend

	httpServer   *http.Server
	router       *chi.Mux
	publicRouter *chi.Mux

	foreman *Foreman

	config *Config

	waitGroup *sync.WaitGroup
	stopChan  chan bool
	stopped   bool

	chanRoutes []string // used for index page
}

func (s *server) initializeChannelHandlers() {
	includes := s.config.IncludeChannels
	excludes := s.config.ExcludeChannels

	// initialize handlers which are included/not-excluded in the config
	for _, handler := range registeredHandlers {
		channelType := string(handler.ChannelType())
		if (includes == nil || utils.StringArrayContains(includes, channelType)) && (excludes == nil || !utils.StringArrayContains(excludes, channelType)) {
			err := handler.Initialize(s)
			if err != nil {
				log.Fatal(err)
			}
			activeHandlers[handler.ChannelType()] = handler

			slog.Info("handler initialized", "comp", "server", "handler", handler.ChannelName(), "handler_type", channelType)
		}
	}

	// sort our route help
	sort.Strings(s.chanRoutes)
}

func (s *server) channelHandleWrapper(handler ChannelHandler, handlerFunc ChannelHandleFunc, logType ChannelLogType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// stuff a few things in our context that help with logging
		baseCtx := context.WithValue(r.Context(), contextRequestURL, r.URL.String())
		baseCtx = context.WithValue(baseCtx, contextRequestStart, time.Now())

		// add a 30 second timeout to the request
		ctx, cancel := context.WithTimeout(baseCtx, time.Second*30)
		defer cancel()
		r = r.WithContext(ctx)

		recorder, err := httpx.NewRecorder(r, w, true)
		if err != nil {
			writeAndLogRequestError(ctx, handler, w, r, nil, err)
			return
		}

		// get the channel for this request - can be nil, e.g. FBA verification requests
		channel, err := handler.GetChannel(ctx, r)
		if err != nil {
			writeAndLogRequestError(ctx, handler, recorder.ResponseWriter, r, channel, err)
			return
		}

		var channelUUID ChannelUUID
		if channel != nil {
			channelUUID = channel.UUID()
		}

		defer func() {
			// catch any panics and recover
			panicLog := recover()
			if panicLog != nil {
				debug.PrintStack()
				slog.Error("panic handling request", "error", err, "channel_uuid", channelUUID, "request", recorder.Trace.RequestTrace, "trace", panicLog)
				writeAndLogRequestError(ctx, handler, recorder.ResponseWriter, r, channel, errors.New("panic handling msg"))
			}
		}()

		clog := NewChannelLogForIncoming(logType, channel, recorder, handler.RedactValues(channel))

		events, hErr := handlerFunc(ctx, channel, recorder.ResponseWriter, r, clog)
		duration := time.Since(start)
		secondDuration := float64(duration) / float64(time.Second)

		// if we received an error, write it out and report it
		if hErr != nil {
			slog.Error("error handling request", "error", err, "channel_uuid", channelUUID, "request", recorder.Trace.RequestTrace)
			writeAndLogRequestError(ctx, handler, recorder.ResponseWriter, r, channel, hErr)
		}

		// end recording of the request so that we have a response trace
		if err := recorder.End(); err != nil {
			slog.Error("error recording request", "error", err, "channel_uuid", channelUUID, "request", recorder.Trace.RequestTrace)
			writeAndLogRequestError(ctx, handler, w, r, channel, err)
		}

		if channel != nil {
			// if we have a channel but no events were created, we still log this to analytics
			if len(events) == 0 {
				if hErr != nil {
					analytics.Gauge(fmt.Sprintf("courier.channel_error_%s", channel.ChannelType()), secondDuration)
				} else {
					analytics.Gauge(fmt.Sprintf("courier.channel_ignored_%s", channel.ChannelType()), secondDuration)
				}
			}

			for _, event := range events {
				switch e := event.(type) {
				case MsgIn:
					clog.SetAttached(true)
					analytics.Gauge(fmt.Sprintf("courier.msg_receive_%s", channel.ChannelType()), secondDuration)
					LogMsgReceived(r, e)
				case StatusUpdate:
					clog.SetAttached(true)
					analytics.Gauge(fmt.Sprintf("courier.msg_status_%s", channel.ChannelType()), secondDuration)
					LogMsgStatusReceived(r, e)
				case ChannelEvent:
					analytics.Gauge(fmt.Sprintf("courier.evt_receive_%s", channel.ChannelType()), secondDuration)
					LogChannelEventReceived(r, e)
				}
			}

			clog.End()

			if err := s.backend.WriteChannelLog(ctx, clog); err != nil {
				slog.Error("error writing channel log", "error", err)
			}
		} else {
			slog.Info("non-channel specific request", "error", err, "channel_type", handler.ChannelType(), "request", recorder.Trace.RequestTrace, "status", recorder.Trace.Response.StatusCode)

		}
	}
}

func (s *server) AddHandlerRoute(handler ChannelHandler, method string, action string, logType ChannelLogType, handlerFunc ChannelHandleFunc) {
	method = strings.ToLower(method)
	channelType := strings.ToLower(string(handler.ChannelType()))

	path := fmt.Sprintf("/%s/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", channelType)
	if !handler.UseChannelRouteUUID() {
		path = fmt.Sprintf("/%s", channelType)
	}

	if action != "" {
		path = fmt.Sprintf("%s/%s", path, action)
	}
	s.publicRouter.Method(method, path, s.channelHandleWrapper(handler, handlerFunc, logType))
	s.chanRoutes = append(s.chanRoutes, fmt.Sprintf("%-20s - %s %s", "/c"+path, handler.ChannelName(), action))
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("<html><head><title>courier</title></head><body><pre>\n")
	buf.WriteString(splash)
	buf.WriteString(s.config.Version)
	buf.WriteString(s.backend.Health())
	buf.WriteString("\n\n")
	buf.WriteString(strings.Join(s.chanRoutes, "\n"))
	buf.WriteString("</pre></body></html>")
	w.Write(buf.Bytes())
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("<html><head><title>courier</title></head><body><pre>\n")
	buf.WriteString(splash)
	buf.WriteString(s.config.Version)
	buf.WriteString("\n\n")
	buf.WriteString(s.backend.Status())
	buf.WriteString("\n\n")
	buf.WriteString("</pre></body></html>")
	w.Write(buf.Bytes())
}

func (s *server) handleFetchAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	resp, err := fetchAttachment(ctx, s.backend, r)
	if err != nil {
		slog.Error("error fetching attachment", "error", err)
		WriteError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}

func (s *server) handle404(w http.ResponseWriter, r *http.Request) {
	slog.Info("not found", "url", r.URL.String(), "method", r.Method, "resp_status", "404")
	errors := []any{NewErrorData(fmt.Sprintf("not found: %s", r.URL.String()))}
	err := WriteDataResponse(w, http.StatusNotFound, "Not Found", errors)
	if err != nil {
		slog.Error("error writing response", "error", err)
	}
}

func (s *server) handle405(w http.ResponseWriter, r *http.Request) {
	slog.Info("invalid method", "url", r.URL.String(), "method", r.Method, "resp_status", "405")
	errors := []any{NewErrorData(fmt.Sprintf("method not allowed: %s", r.Method))}
	err := WriteDataResponse(w, http.StatusMethodNotAllowed, "Method Not Allowed", errors)
	if err != nil {
		slog.Error("error writing response", "error", err)

	}
}

// wraps a handler to make it use basic auth
func (s *server) basicAuthRequired(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.StatusUsername != "" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != s.config.StatusUsername || pass != s.config.StatusPassword {
				w.Header().Set("WWW-Authenticate", `Basic realm="Authenticate"`)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
				return
			}
		}
		h(w, r)
	}
}

// wraps a handler to make it use token auth
func (s *server) tokenAuthRequired(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") || authHeader[7:] != s.config.AuthToken {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		h(w, r)
	}
}

var splash = `
 ____________                   _____             
   ___  ____/_________  ___________(_)____________
    _  /  __  __ \  / / /_  ___/_  /_  _ \_  ___/
    / /__  / /_/ / /_/ /_  /   _  / /  __/  /    
    \____/ \____/\__,_/ /_/    /_/  \___//_/ v`
