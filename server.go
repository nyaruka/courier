package courier

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/courier/v26/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

// for use in request.Context
type contextKey int

const (
	contextRequestURL contextKey = iota
	contextRequestStart
)

// NewServer creates a new Server for the passed in runtime. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServer(rt *runtime.Runtime, backend Backend) *Server {
	// channelRouter holds the dynamically-registered channel handler routes - mounted at /c/ on the public listener
	channelRouter := chi.NewRouter()

	// testRouter mounts channelRouter at /c/ so handler tests can dispatch requests via Router() without
	// spinning up the listener. It mirrors the public listener's middleware stack so tests exercise the
	// same chain that /c/* traffic hits in production.
	testRouter := chi.NewRouter()
	testRouter.Use(middleware.Compress(flate.DefaultCompression))
	testRouter.Use(middleware.StripSlashes)
	testRouter.Use(middleware.RequestID)
	testRouter.Use(middleware.RealIP)
	testRouter.Use(middleware.Recoverer)
	testRouter.Use(middleware.Timeout(30 * time.Second))
	testRouter.Mount("/c/", channelRouter)

	return &Server{
		rt:      rt,
		backend: backend,

		channelRouter: channelRouter,
		testRouter:    testRouter,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},
		stopped:   false,
	}
}

// Start starts the Server listening for incoming requests and sending messages. It will return an error
// if it encounters any unrecoverable (or ignorable) error, though its bias is to move forward despite
// connection errors
func (s *Server) Start() error {
	// bind both listener sockets up front so callers know we're accepting connections by the
	// time Start returns, and so a bind failure fails fast before we've started the backend,
	// spool flushers, or anything else that would need to be unwound
	publicAddr := fmt.Sprintf("%s:%d", s.rt.Config.PublicAddress, s.rt.Config.PublicPort)
	publicLn, err := net.Listen("tcp", publicAddr)
	if err != nil {
		return fmt.Errorf("error binding public listener on %s: %w", publicAddr, err)
	}
	internalAddr := fmt.Sprintf("%s:%d", s.rt.Config.InternalAddress, s.rt.Config.InternalPort)
	internalLn, err := net.Listen("tcp", internalAddr)
	if err != nil {
		publicLn.Close()
		return fmt.Errorf("error binding internal listener on %s: %w", internalAddr, err)
	}

	// start our backend
	if err := s.backend.Start(); err != nil {
		publicLn.Close()
		internalLn.Close()
		return err
	}

	// start our spool flushers
	startSpoolFlushers(s)

	// initialize our handlers (wires routes into channelRouter)
	s.initializeChannelHandlers()

	// public listener — exposes /c/*, /
	publicRouter := chi.NewRouter()
	publicRouter.Use(middleware.Compress(flate.DefaultCompression))
	publicRouter.Use(middleware.StripSlashes)
	publicRouter.Use(middleware.RequestID)
	publicRouter.Use(middleware.RealIP)
	publicRouter.Use(middleware.Recoverer)
	publicRouter.Use(middleware.Timeout(30 * time.Second))
	publicRouter.NotFound(s.handle404("public"))
	publicRouter.MethodNotAllowed(s.handle405("public"))
	publicRouter.Get("/", s.handleHealth)
	publicRouter.Mount("/c/", s.channelRouter)

	// internal listener — only /ci/* routes and /, no public-facing concerns
	internalRouter := chi.NewRouter()
	internalRouter.Use(middleware.Compress(flate.DefaultCompression))
	internalRouter.Use(middleware.StripSlashes)
	internalRouter.Use(middleware.RequestID)
	internalRouter.Use(middleware.Recoverer)
	internalRouter.Use(middleware.Timeout(30 * time.Second))
	internalRouter.NotFound(s.handle404("internal"))
	internalRouter.MethodNotAllowed(s.handle405("internal"))
	internalRouter.Get("/", s.handleHealth)
	internalRouter.Post("/ci/attachment/fetch", s.tokenAuthRequired(s.handleFetchAttachment))
	internalRouter.Post("/ci/event/send", s.tokenAuthRequired(s.handleSendEvent))

	s.publicServer = &http.Server{
		Addr:         publicAddr,
		Handler:      publicRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  90 * time.Second,
	}
	s.internalServer = &http.Server{
		Addr:         internalAddr,
		Handler:      internalRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	s.waitGroup.Add(2)

	go func() {
		defer s.waitGroup.Done()

		log := slog.With("comp", "server", "listener", "public", "address", s.publicServer.Addr)
		log.Info("server started", "version", s.rt.Config.Version)

		err := s.publicServer.Serve(publicLn)
		if err != nil && err != http.ErrServerClosed {
			log.Error("error listening", "error", err)
		}
	}()

	go func() {
		defer s.waitGroup.Done()

		log := slog.With("comp", "server", "listener", "internal", "address", s.internalServer.Addr)
		log.Info("server started", "version", s.rt.Config.Version)

		err := s.internalServer.Serve(internalLn)
		if err != nil && err != http.ErrServerClosed {
			log.Error("error listening", "error", err)
		}
	}()

	// start our foreman for outgoing messages
	s.foreman = NewForeman(s, s.rt.Config.MaxWorkers)
	s.foreman.Start()

	return nil
}

// Stop stops the server, returning only after all threads have stopped
func (s *Server) Stop() error {
	log := slog.With("comp", "server")
	log.Info("stopping server", "state", "stopping")

	// stop our foreman
	s.foreman.Stop()

	// shut down both HTTP servers
	if err := s.publicServer.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down server", "listener", "public", "error", err, "state", "stopping")
	}
	if err := s.internalServer.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down server", "listener", "internal", "error", err, "state", "stopping")
	}

	// stop everything
	s.stopped = true
	close(s.stopChan)

	// stop our backend
	if err := s.backend.Stop(); err != nil {
		return err
	}

	// wait for everything to stop
	s.waitGroup.Wait()

	log.Info("server stopped", "state", "stopped")
	return nil
}

func (s *Server) GetHandler(ch Channel) ChannelHandler { return activeHandlers[ch.ChannelType()] }

func (s *Server) WaitGroup() *sync.WaitGroup { return s.waitGroup }
func (s *Server) StopChan() chan bool        { return s.stopChan }
func (s *Server) Runtime() *runtime.Runtime  { return s.rt }
func (s *Server) Stopped() bool              { return s.stopped }

func (s *Server) Backend() Backend   { return s.backend }
func (s *Server) Router() chi.Router { return s.testRouter }

type Server struct {
	backend Backend

	publicServer   *http.Server
	internalServer *http.Server
	channelRouter  *chi.Mux
	testRouter     *chi.Mux

	foreman *Foreman

	rt *runtime.Runtime

	waitGroup *sync.WaitGroup
	stopChan  chan bool
	stopped   bool
}

func (s *Server) initializeChannelHandlers() {
	includes := s.rt.Config.IncludeChannels
	excludes := s.rt.Config.ExcludeChannels

	// initialize handlers which are included/not-excluded in the config
	for _, handler := range registeredHandlers {
		channelType := string(handler.ChannelType())
		if (includes == nil || slices.Contains(includes, channelType)) && (excludes == nil || !slices.Contains(excludes, channelType)) {
			err := handler.Initialize(s)
			if err != nil {
				log.Fatal(err)
			}
			activeHandlers[handler.ChannelType()] = handler

			slog.Info("handler initialized", "comp", "server", "handler", handler.ChannelName(), "handler_type", channelType)
		}
	}

}

func (s *Server) channelHandleWrapper(handler ChannelHandler, handlerFunc ChannelHandleFunc, logType clogs.Type) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		var channelUUID models.ChannelUUID
		if channel != nil {
			channelUUID = channel.UUID()
		}

		defer func() {
			// catch any panics and recover
			if panicVal := recover(); panicVal != nil {
				debug.PrintStack()

				sentry.CurrentHub().Recover(panicVal)

				writeAndLogRequestError(ctx, handler, recorder.ResponseWriter, r, channel, errors.New("panic handling msg"))
			}
		}()

		clog := NewChannelLogForIncoming(logType, channel, recorder, handler.RedactValues(channel))

		events, hErr := handlerFunc(ctx, channel, recorder.ResponseWriter, r, clog)

		// if we received an error, write it out and report it
		if hErr != nil {
			slog.Error("error handling request", "error", hErr, "channel", channelUUID, "url", recorder.Trace.Request.URL.String())
			writeAndLogRequestError(ctx, handler, recorder.ResponseWriter, r, channel, hErr)
		}

		// end recording of the request so that we have a response trace
		if err := recorder.End(); err != nil {
			slog.Error("error recording request", "error", err, "channel", channelUUID)
			writeAndLogRequestError(ctx, handler, w, r, channel, err)
		}

		if channel != nil {
			for _, event := range events {
				switch e := event.(type) {
				case MsgIn:
					LogMsgReceived(r, e)
				case StatusUpdate:
					LogMsgStatusReceived(r, e)
				case ChannelEvent:
					LogChannelEventReceived(r, e)
				}
			}

			clog.End()

			if err := s.backend.WriteChannelLog(ctx, clog); err != nil {
				slog.Error("error writing channel log", "error", err)
			}

			s.backend.OnReceiveComplete(ctx, channel, events, clog)
		} else {
			slog.Info("non-channel specific request", "error", err, "channel_type", handler.ChannelType(), "request", recorder.Trace.RequestTrace, "status", recorder.Trace.Response.StatusCode)
		}
	}
}

func (s *Server) AddHandlerRoute(handler ChannelHandler, method string, action string, logType clogs.Type, handlerFunc ChannelHandleFunc) {
	method = strings.ToLower(method)
	channelType := strings.ToLower(string(handler.ChannelType()))

	path := fmt.Sprintf("/%s/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", channelType)
	if !handler.UseChannelRouteUUID() {
		path = fmt.Sprintf("/%s", channelType)
	}

	if action != "" {
		path = fmt.Sprintf("%s/%s", path, action)
	}
	s.channelRouter.Method(method, path, s.channelHandleWrapper(handler, handlerFunc, logType))
}

func (s *Server) handleFetchAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	resp, err := fetchAttachment(ctx, s.rt, s.backend, r)
	if err != nil {
		slog.Error("error fetching attachment", "error", err)
		WriteError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}

func (s *Server) handleSendEvent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	resp, err := sendEvent(ctx, s.backend, r)
	if err != nil {
		slog.Error("error sending event", "error", err)
		WriteError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}

// handle404 returns a 404 handler. The internal listener logs at Error level (sentry-routed via slog-sentry)
// so we alert on caller-side bugs in rapidpro/mailroom that hit unknown internal paths.
func (s *Server) handle404(listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if listener == "internal" {
			slog.Error("not found", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "404")
		} else {
			slog.Info("not found", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "404")
		}
		errors := []any{NewErrorData(fmt.Sprintf("not found: %s", r.URL.String()))}
		err := WriteDataResponse(w, http.StatusNotFound, "Not Found", errors)
		if err != nil {
			slog.Error("error writing response", "error", err)
		}
	}
}

func (s *Server) handle405(listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if listener == "internal" {
			slog.Error("invalid method", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "405")
		} else {
			slog.Info("invalid method", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "405")
		}
		errors := []any{NewErrorData(fmt.Sprintf("method not allowed: %s", r.Method))}
		err := WriteDataResponse(w, http.StatusMethodNotAllowed, "Method Not Allowed", errors)
		if err != nil {
			slog.Error("error writing response", "error", err)
		}
	}
}

// handleHealth is the liveness probe used by ALB health checks. Registered at the root of
// both listeners and not under any /c or /ci prefix, so no listener rule routes client traffic
// to it — only direct ALB→target health probes reach it. Also returns the running version so
// it doubles as a debug endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(map[string]string{
		"component": "courier",
		"version":   s.rt.Config.Version,
	}))
}

// wraps a handler to make it use token auth
func (s *Server) tokenAuthRequired(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") || !utils.SecretEqual(authHeader[7:], s.rt.Config.AuthToken) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		h(w, r)
	}
}
