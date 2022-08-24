package courier

import (
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"log"
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
	"github.com/sirupsen/logrus"
)

// Server is the main interface ChannelHandlers use to interact with backends. It provides an
// abstraction that makes mocking easier for isolated unit tests
type Server interface {
	Config() *Config

	AddHandlerRoute(handler ChannelHandler, method string, action string, handlerFunc ChannelHandleFunc)

	SendMsg(context.Context, Msg, *ChannelLogger) (MsgStatus, error)

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
	logger := logrus.New()
	return NewServerWithLogger(config, backend, logger)
}

// NewServerWithLogger creates a new Server for the passed in configuration. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServerWithLogger(config *Config, backend Backend, logger *logrus.Logger) Server {
	router := chi.NewRouter()
	router.Use(middleware.Compress(flate.DefaultCompression))
	router.Use(middleware.StripSlashes)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))

	chanRouter := chi.NewRouter()
	router.Mount("/c/", chanRouter)

	return &server{
		config:  config,
		backend: backend,

		router:     router,
		chanRouter: chanRouter,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},
		stopped:   false,
	}
}

// Start starts the Server listening for incoming requests and sending messages. It will return an error
// if it encounters any unrecoverable (or ignorable) error, though its bias is to move forward despite
// connection errors
func (s *server) Start() error {
	// set our user agent, needs to happen before we do anything so we don't change have threading issues
	utils.HTTPUserAgent = fmt.Sprintf("Courier/%s", s.config.Version)

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
	s.router.Get("/status", s.handleStatus)

	// initialize our handlers
	s.initializeChannelHandlers()

	// configure timeouts on our server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Address, s.config.Port),
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	s.waitGroup.Add(1)

	// and start serving HTTP
	go func() {
		defer s.waitGroup.Done()
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logrus.WithFields(logrus.Fields{
				"comp":  "server",
				"state": "stopping",
				"err":   err,
			}).Error()
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
					logrus.WithError(err).Error("error running backend heartbeat")
				}
			}
		}
	}()

	logrus.WithFields(logrus.Fields{
		"comp":    "server",
		"port":    s.config.Port,
		"state":   "started",
		"version": s.config.Version,
	}).Info("server listening on ", s.config.Port)

	// start our foreman for outgoing messages
	s.foreman = NewForeman(s, s.config.MaxWorkers)
	s.foreman.Start()

	return nil
}

// Stop stops the server, returning only after all threads have stopped
func (s *server) Stop() error {
	log := logrus.WithField("comp", "server")
	log.WithField("state", "stopping").Info("stopping server")

	// stop our foreman
	s.foreman.Stop()

	// shut down our HTTP server
	if err := s.httpServer.Shutdown(context.Background()); err != nil {
		log.WithField("state", "stopping").WithError(err).Error("error shutting down server")
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

	log.WithField("state", "stopped").Info("server stopped")
	return nil
}

func (s *server) SendMsg(ctx context.Context, msg Msg, logger *ChannelLogger) (MsgStatus, error) {
	// find the handler for this message type
	handler, found := activeHandlers[msg.Channel().ChannelType()]
	if !found {
		return nil, fmt.Errorf("unable to find handler for channel type: %s", msg.Channel().ChannelType())
	}

	// have the handler send it
	return handler.Send(ctx, msg, logger)
}

func (s *server) WaitGroup() *sync.WaitGroup { return s.waitGroup }
func (s *server) StopChan() chan bool        { return s.stopChan }
func (s *server) Config() *Config            { return s.config }
func (s *server) Stopped() bool              { return s.stopped }

func (s *server) Backend() Backend   { return s.backend }
func (s *server) Router() chi.Router { return s.router }

type server struct {
	backend Backend

	httpServer *http.Server
	router     *chi.Mux
	chanRouter *chi.Mux

	foreman *Foreman

	config *Config

	waitGroup *sync.WaitGroup
	stopChan  chan bool
	stopped   bool

	routes []string
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

			logrus.WithField("comp", "server").WithField("handler", handler.ChannelName()).WithField("handler_type", channelType).Info("handler initialized")
		}
	}

	// sort our route help
	sort.Strings(s.routes)
}

func (s *server) channelHandleWrapper(handler ChannelHandler, handlerFunc ChannelHandleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// stuff a few things in our context that help with logging
		baseCtx := context.WithValue(r.Context(), contextRequestURL, r.URL.String())
		baseCtx = context.WithValue(baseCtx, contextRequestStart, time.Now())

		// add a 30 second timeout to the request
		ctx, cancel := context.WithTimeout(baseCtx, time.Second*30)
		defer cancel()
		r = r.WithContext(ctx)

		// get the channel for this request - can be nil, e.g. FBA verification requests
		channel, err := handler.GetChannel(ctx, r)
		if err != nil {
			WriteError(ctx, w, r, err)
			return
		}

		recorder, err := httpx.NewRecorder(r, w, true)
		if err != nil {
			writeAndLogRequestError(ctx, w, r, channel, err)
			return
		}

		defer func() {
			// catch any panics and recover
			panicLog := recover()
			if panicLog != nil {
				debug.PrintStack()
				logrus.WithError(err).WithField("channel_uuid", channel.UUID()).WithField("request", string(recorder.Trace.RequestTrace)).WithField("trace", panicLog).Error("panic handling request")
				writeAndLogRequestError(ctx, recorder.ResponseWriter, r, channel, errors.New("panic handling msg"))
			}
		}()

		logger := NewChannelLogger(ChannelLogTypeUnknown, channel)

		events, hErr := handlerFunc(ctx, channel, recorder.ResponseWriter, r, logger)
		duration := time.Since(start)
		secondDuration := float64(duration) / float64(time.Second)

		// if we received an error, write it out and report it
		if hErr != nil {
			logrus.WithError(hErr).WithField("channel_uuid", channel.UUID()).WithField("request", string(recorder.Trace.RequestTrace)).Error("error handling request")
			writeAndLogRequestError(ctx, recorder.ResponseWriter, r, channel, hErr)
		}

		// end recording of the request so that we have a response trace
		if err := recorder.End(); err != nil {
			logrus.WithError(err).WithField("channel_uuid", channel.UUID()).WithField("request", string(recorder.Trace.RequestTrace)).Error("error receording request")
			writeAndLogRequestError(ctx, w, r, channel, err)
		}

		logger.Recorder(recorder)

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
				case Msg:
					logger.SetMsgID(e.ID())
					logger.SetType(ChannelLogTypeMsgReceive)
					analytics.Gauge(fmt.Sprintf("courier.msg_receive_%s", channel.ChannelType()), secondDuration)
					LogMsgReceived(r, e)
				case MsgStatus:
					logger.SetMsgID(e.ID())
					logger.SetType(ChannelLogTypeMsgStatus)
					analytics.Gauge(fmt.Sprintf("courier.msg_status_%s", channel.ChannelType()), secondDuration)
					LogMsgStatusReceived(r, e)
				case ChannelEvent:
					logger.SetType(ChannelLogTypeEventReceive)
					analytics.Gauge(fmt.Sprintf("courier.evt_receive_%s", channel.ChannelType()), secondDuration)
					LogChannelEventReceived(r, e)
				}
			}

			if err := s.backend.WriteChannelLogs(ctx, logger.Logs()); err != nil {
				logrus.WithError(err).Error("error writing channel log")
			}
		}
	}
}

func (s *server) AddHandlerRoute(handler ChannelHandler, method string, action string, handlerFunc ChannelHandleFunc) {
	method = strings.ToLower(method)
	channelType := strings.ToLower(string(handler.ChannelType()))

	path := fmt.Sprintf("/%s/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}", channelType)
	if !handler.UseChannelRouteUUID() {
		path = fmt.Sprintf("/%s", channelType)
	}

	if action != "" {
		path = fmt.Sprintf("%s/%s", path, action)
	}
	s.chanRouter.Method(method, path, s.channelHandleWrapper(handler, handlerFunc))
	s.routes = append(s.routes, fmt.Sprintf("%-20s - %s %s", "/c"+path, handler.ChannelName(), action))
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {

	var buf bytes.Buffer
	buf.WriteString("<title>courier</title><body><pre>\n")
	buf.WriteString(splash)
	buf.WriteString(s.config.Version)

	buf.WriteString(s.backend.Health())

	buf.WriteString("\n\n")
	buf.WriteString(strings.Join(s.routes, "\n"))
	buf.WriteString("</pre></body>")
	w.Write(buf.Bytes())
}

func (s *server) handle404(w http.ResponseWriter, r *http.Request) {
	logrus.WithField("url", r.URL.String()).WithField("method", r.Method).WithField("resp_status", "404").Info("not found")
	errors := []interface{}{NewErrorData(fmt.Sprintf("not found: %s", r.URL.String()))}
	err := WriteDataResponse(context.Background(), w, http.StatusNotFound, "Not Found", errors)
	if err != nil {
		logrus.WithError(err).Error()
	}
}

func (s *server) handle405(w http.ResponseWriter, r *http.Request) {
	logrus.WithField("url", r.URL.String()).WithField("method", r.Method).WithField("resp_status", "405").Info("invalid method")
	errors := []interface{}{NewErrorData(fmt.Sprintf("method not allowed: %s", r.Method))}
	err := WriteDataResponse(context.Background(), w, http.StatusMethodNotAllowed, "Method Not Allowed", errors)
	if err != nil {
		logrus.WithError(err).Error()
	}
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.config.StatusUsername != "" {
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.config.StatusUsername || pass != s.config.StatusPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Authenticate"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorised.\n"))
			return
		}
	}

	var buf bytes.Buffer
	buf.WriteString("<title>courier</title><body><pre>\n")
	buf.WriteString(splash)
	buf.WriteString(s.config.Version)

	buf.WriteString("\n\n")
	buf.WriteString(s.backend.Status())
	buf.WriteString("\n\n")
	buf.WriteString("</pre></body>")
	w.Write(buf.Bytes())
}

// for use in request.Context
type contextKey int

const (
	contextRequestURL contextKey = iota
	contextRequestStart
)

var splash = `
 ____________                   _____             
   ___  ____/_________  ___________(_)____________
    _  /  __  __ \  / / /_  ___/_  /_  _ \_  ___/
    / /__  / /_/ / /_/ /_  /   _  / /  __/  /    
    \____/ \____/\__,_/ /_/    /_/  \___//_/ v`
