package web

import (
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"github.com/nyaruka/courier/v26/core/channels"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
)

// NewServer creates a new Server for the passed in runtime. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServer(rt *runtime.Runtime) *Server {
	// channelRouter holds the dynamically-registered channel handler routes - mounted at /c/ on the internet listener
	channelRouter := chi.NewRouter()

	// testRouter mounts channelRouter at /c/ so handler tests can dispatch requests via Router() without
	// spinning up the listener. It mirrors the internet listener's middleware stack so tests exercise the
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
		rt: rt,

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
	// time Start returns, and so a bind failure fails fast before the serve goroutines start
	internetAddr := fmt.Sprintf("%s:%d", s.rt.Config.InternetAddress, s.rt.Config.InternetPort)
	internetLn, err := net.Listen("tcp", internetAddr)
	if err != nil {
		return fmt.Errorf("error binding internet listener on %s: %w", internetAddr, err)
	}
	internalAddr := fmt.Sprintf("%s:%d", s.rt.Config.InternalAddress, s.rt.Config.InternalPort)
	internalLn, err := net.Listen("tcp", internalAddr)
	if err != nil {
		internetLn.Close()
		return fmt.Errorf("error binding internal listener on %s: %w", internalAddr, err)
	}

	// mount the routes of every handler this instance serves
	if err := s.mountChannelHandlers(); err != nil {
		internetLn.Close()
		internalLn.Close()
		return err
	}

	// internet listener — exposes /c/*, /
	internetRouter := chi.NewRouter()
	internetRouter.Use(middleware.Compress(flate.DefaultCompression))
	internetRouter.Use(middleware.StripSlashes)
	internetRouter.Use(middleware.RequestID)
	internetRouter.Use(middleware.RealIP)
	internetRouter.Use(middleware.Recoverer)
	internetRouter.Use(middleware.Timeout(30 * time.Second))
	internetRouter.NotFound(s.handle404("internet"))
	internetRouter.MethodNotAllowed(s.handle405("internet"))
	internetRouter.Get("/", s.handleHealth("internet"))
	internetRouter.Mount("/c/", s.channelRouter)

	// internal listener — only /ci/* routes and /, no internet-facing concerns
	internalRouter := chi.NewRouter()
	internalRouter.Use(middleware.Compress(flate.DefaultCompression))
	internalRouter.Use(middleware.StripSlashes)
	internalRouter.Use(middleware.RequestID)
	internalRouter.Use(middleware.Recoverer)
	internalRouter.Use(middleware.Timeout(30 * time.Second))
	internalRouter.NotFound(s.handle404("internal"))
	internalRouter.MethodNotAllowed(s.handle405("internal"))
	internalRouter.Get("/", s.handleHealth("internal"))
	internalRouter.Post("/ci/attachment/fetch", s.tokenAuthRequired(s.handleFetchAttachment))
	internalRouter.Post("/ci/event/send", s.tokenAuthRequired(s.handleSendEvent))

	s.internetServer = &http.Server{
		Addr:         internetAddr,
		Handler:      internetRouter,
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

		log := slog.With("comp", "server", "listener", "internet", "address", s.internetServer.Addr)
		log.Info("server started", "version", s.rt.Config.Version)

		err := s.internetServer.Serve(internetLn)
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

	return nil
}

// Stop stops the server, returning only after all threads have stopped
func (s *Server) Stop() error {
	log := slog.With("comp", "server")
	log.Info("stopping server", "state", "stopping")

	// shut down both HTTP servers
	if err := s.internetServer.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down server", "listener", "internet", "error", err, "state", "stopping")
	}
	if err := s.internalServer.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down server", "listener", "internal", "error", err, "state", "stopping")
	}

	s.stopped = true
	close(s.stopChan)

	// wait for both listeners to finish serving
	s.waitGroup.Wait()

	log.Info("server stopped", "state", "stopped")
	return nil
}

func (s *Server) Runtime() *runtime.Runtime { return s.rt }
func (s *Server) Stopped() bool             { return s.stopped }

func (s *Server) Router() chi.Router { return s.testRouter }

// OnRequestHandled sets a hook called after each channel request is handled, with the events it produced and its
// channel log - used by tests to capture what handlers return.
func (s *Server) OnRequestHandled(fn func(*models.Channel, []channels.Event, *models.ChannelLog)) {
	s.requestHandled = fn
}

type Server struct {
	internetServer *http.Server
	internalServer *http.Server
	channelRouter  *chi.Mux
	testRouter     *chi.Mux

	rt *runtime.Runtime

	requestHandled func(*models.Channel, []channels.Event, *models.ChannelLog)

	waitGroup *sync.WaitGroup
	stopChan  chan bool
	stopped   bool
}

// mounts the routes of every handler included by config, so that a request for one of its channels reaches it
func (s *Server) mountChannelHandlers() error {
	includes := s.rt.Config.IncludeChannels
	excludes := s.rt.Config.ExcludeChannels

	for _, handler := range channels.RegisteredHandlers() {
		channelType := string(handler.ChannelType())
		if (includes == nil || slices.Contains(includes, channelType)) && (excludes == nil || !slices.Contains(excludes, channelType)) {
			if err := s.MountHandler(handler); err != nil {
				return err
			}

			slog.Info("handler initialized", "comp", "server", "handler", handler.ChannelName(), "handler_type", channelType)
		}
	}

	return nil
}

// MountHandler initializes the given handler and mounts the routes it registers, marking it as one this instance
// serves. Tests use it to mount a single handler without starting the listeners.
func (s *Server) MountHandler(handler channels.Handler) error {
	handler.SetRuntime(s.rt)

	routes := channels.NewRoutes()
	if err := handler.Initialize(routes); err != nil {
		return fmt.Errorf("error initializing handler %s: %w", handler.ChannelType(), err)
	}

	for _, route := range routes.All() {
		s.addRoute(route)
	}

	channels.ActivateHandler(handler)
	return nil
}

func (s *Server) channelHandleWrapper(handler channels.Handler, handlerFunc channels.HandleFunc, logType svclogs.Type) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// stuff a few things in our context that help with logging
		baseCtx := channels.WithRequestContext(r.Context(), r.URL.String(), start)

		// add a 30 second timeout to the request
		ctx, cancel := context.WithTimeout(baseCtx, time.Second*30)
		defer cancel()
		r = r.WithContext(ctx)

		recorder, err := httpx.NewRecorder(r, w, true)
		if err != nil {
			channels.WriteErrorResponse(ctx, handler, w, r, nil, err)
			return
		}

		// get the channel for this request - can be nil, e.g. FBA verification requests
		channel, err := handler.GetChannel(ctx, r)
		if err != nil {
			channels.WriteErrorResponse(ctx, handler, recorder.ResponseWriter, r, channel, err)
			return
		}

		var channelUUID models.ChannelUUID
		if channel != nil {
			channelUUID = channel.UUID()
		}

		defer func() {
			// catch any panics and recover
			if panicVal := recover(); panicVal != nil {
				runtime.PanicHandler(panicVal, map[string]string{"channel_type": string(handler.ChannelType())})

				channels.WriteErrorResponse(ctx, handler, recorder.ResponseWriter, r, channel, errors.New("panic handling msg"))
			}
		}()

		clog := models.NewChannelLogForIncoming(logType, channel, recorder, handler.RedactValues(channel))

		events, hErr := handlerFunc(ctx, channel, recorder.ResponseWriter, r, clog)

		// an error from the handler here is courier's own - the seam answers request-level errors itself - so
		// it's logged as an error above the response, rather than as the provider's problem
		if hErr != nil {
			slog.Error("error handling request", "error", hErr, "channel", channelUUID, "url", recorder.Trace.Request.URL.String())
			handler.WriteRequestError(ctx, recorder.ResponseWriter, hErr)
		}

		// end recording of the request so that we have a response trace
		if err := recorder.End(); err != nil {
			slog.Error("error recording request", "error", err, "channel", channelUUID)
			handler.WriteRequestError(ctx, w, err)
		}

		if channel != nil {
			numMsgs, numStatuses, numEvents, numIgnored := 0, 0, 0, 0

			debugLog := slog.Default().Enabled(ctx, slog.LevelDebug)
			elapsedMS := float64(time.Since(start)) / float64(time.Millisecond)

			for _, event := range events {
				switch e := event.(type) {
				case *models.MsgIn:
					if debugLog {
						slog.Debug("msg received", "channel_uuid", channelUUID, "url", r.URL.String(), "elapsed_ms", elapsedMS,
							"msg_uuid", e.UUID(), "msg_urn", e.URN().Identity(), "msg_text", e.Text(), "msg_attachments", e.Attachments())
					}
					if e.Duplicate_ {
						numIgnored++
					} else {
						numMsgs++
					}
				case *models.StatusUpdate:
					if debugLog {
						slog.Debug("status updated", "channel_uuid", channelUUID, "url", r.URL.String(), "elapsed_ms", elapsedMS,
							"status", e.Status(), "msg_external_id", e.ExternalIdentifier())
					}
					numStatuses++
				case *models.ChannelEvent:
					if debugLog {
						slog.Debug("event received", "channel_uuid", channelUUID, "url", r.URL.String(), "elapsed_ms", elapsedMS,
							"event_type", e.EventType(), "event_urn", e.URN().Identity())
					}
					numEvents++
				}
			}
			if len(events) == 0 {
				numIgnored++
			}

			clog.End()

			if handler.StoreChannelLogs() {
				models.WriteChannelLog(s.rt, clog)
			}

			s.rt.Stats.RecordIncoming(string(channel.ChannelType()), numMsgs, numStatuses, numEvents, numIgnored, clog.Elapsed)

			if s.requestHandled != nil {
				s.requestHandled(channel, events, clog)
			}
		} else {
			slog.Info("non-channel specific request", "error", err, "channel_type", handler.ChannelType(), "request", recorder.Trace.RequestTrace, "status", recorder.Trace.Response.StatusCode)
		}
	}
}

// mounts a route registered by a handler onto the channel router
func (s *Server) addRoute(route *channels.Route) {
	method := strings.ToLower(route.Method)
	channelType := strings.ToLower(string(route.Handler.ChannelType()))

	path := fmt.Sprintf("/%s/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", channelType)
	if !route.Handler.UseChannelRouteUUID() {
		path = fmt.Sprintf("/%s", channelType)
	}

	if route.Action != "" {
		path = fmt.Sprintf("%s/%s", path, route.Action)
	}
	s.channelRouter.Method(method, path, s.channelHandleWrapper(route.Handler, route.Func, route.LogType))
}

func (s *Server) handleFetchAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	resp, err := fetchAttachment(ctx, s.rt, r)
	if err != nil {
		slog.Error("error fetching attachment", "error", err)
		channels.WriteError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}

func (s *Server) handleSendEvent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	resp, err := sendEvent(ctx, s, r)
	if err != nil {
		slog.Error("error sending event", "error", err)
		channels.WriteError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonx.MustMarshal(resp))
}

// handle404 returns a 404 handler. The internal listener logs at Error level so we alert on caller-side bugs in
// rapidpro/mailroom that hit unknown internal paths.
func (s *Server) handle404(listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if listener == "internal" {
			slog.Error("not found", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "404")
		} else {
			slog.Info("not found", "listener", listener, "url", r.URL.String(), "method", r.Method, "resp_status", "404")
		}
		errors := []any{channels.NewErrorData(fmt.Sprintf("not found: %s", r.URL.String()))}
		err := channels.WriteDataResponse(w, http.StatusNotFound, "Not Found", errors)
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
		errors := []any{channels.NewErrorData(fmt.Sprintf("method not allowed: %s", r.Method))}
		err := channels.WriteDataResponse(w, http.StatusMethodNotAllowed, "Method Not Allowed", errors)
		if err != nil {
			slog.Error("error writing response", "error", err)
		}
	}
}

// handleHealth returns the liveness probe used by ALB health checks. Registered at the root of
// both listeners and not under any /c or /ci prefix, so no listener rule routes client traffic
// to it — only direct ALB→target health probes reach it. Also returns the running version and
// which listener was hit so it doubles as a debug endpoint.
func (s *Server) handleHealth(listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonx.MustMarshal(map[string]string{
			"component": "courier",
			"listener":  listener,
			"version":   s.rt.Config.Version,
		}))
	}
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
