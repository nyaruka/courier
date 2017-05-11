package courier

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/courier/config"
)

// Server is the main interface ChannelHandlers use to interact with the database and redis. It provides an
// abstraction that makes mocking easier for isolated unit tests
type Server interface {
	GetConfig() *config.Courier
	AddChannelRoute(handler ChannelHandler, method string, action string, handlerFunc ChannelActionHandlerFunc) *mux.Route
	GetChannel(ChannelType, string) (*Channel, error)

	QueueMsg(*Msg) error
	UpdateMsgStatus(*MsgStatusUpdate) error

	Start() error
	Stop()
}

// ChannelActionHandlerFunc is the interface ChannelHandler functions must satisfy to handle various requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
type ChannelActionHandlerFunc func(*Channel, http.ResponseWriter, *http.Request) error

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(Server) error
	ChannelType() ChannelType
	ChannelName() string
}

// NewServer creates a new Server for the passed in configuration. The server will have to be started
// afterwards, which is when configuration options are checked.
func NewServer(config *config.Courier) Server {
	// create our top level router
	router := mux.NewRouter()
	chanRouter := router.PathPrefix("/c/").Subrouter()

	return &server{
		config: config,

		router:     router,
		chanRouter: chanRouter,

		stopChan:  make(chan bool),
		waitGroup: &sync.WaitGroup{},
	}
}

// Start starts the Server listening for incoming requests and sending messages. It will return an error
// if it encounters any unrecoverable (or ignorable) error, though its bias is to move forward despite
// connection errors
func (s *server) Start() error {
	// parse and test our db config
	dbURL, err := url.Parse(s.config.DB)
	if err != nil {
		return fmt.Errorf("unable to parse DB URL '%s': %s", s.config.DB, err)
	}

	if dbURL.Scheme != "postgres" {
		return fmt.Errorf("invalid DB URL: '%s', only postgres is supported", s.config.DB)
	}

	fmt.Println(splash)

	// test our db connection
	db, err := sqlx.Connect("postgres", s.config.DB)
	if err != nil {
		log.Printf("[ ] DB: error connecting: %s\n", err)
	} else {
		log.Println("[X] DB: connection ok")
	}
	s.db = db

	// parse and test our redis config
	redisURL, err := url.Parse(s.config.Redis)
	if err != nil {
		return fmt.Errorf("unable to parse Redis URL '%s': %s", s.config.Redis, err)
	}

	// create our pool
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   5,                 // only open this many concurrent connections at once
		MaxIdle:     2,                 // only keep up to 2 idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", fmt.Sprintf("%s", redisURL.Host))
			if err != nil {
				return nil, err
			}

			// switch to the right DB
			_, err = conn.Do("SELECT", strings.TrimLeft(redisURL.Path, "/"))
			return conn, err
		},
	}
	s.redisPool = redisPool

	// test our redis connection
	conn := redisPool.Get()
	defer conn.Close()
	_, err = conn.Do("PING")
	if err != nil {
		log.Printf("[ ] Redis: error connecting: %s\n", err)
	} else {
		log.Println("[X] Redis: connection ok")
	}

	// create our s3 client
	s3Session, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(s.config.AWS_Access_Key_ID, s.config.AWS_Secret_Access_Key, ""),
		Region:      aws.String(s.config.S3_Region),
	})
	if err != nil {
		return err
	}
	s.s3Client = s3.New(s3Session)

	// test out our S3 credentials
	err = testS3(s)
	if err != nil {
		log.Printf("[ ] S3: bucket inaccessible, media may not save: %s\n", err)
	} else {
		log.Println("[X] S3: bucket accessible")
	}

	// make sure our spool dirs are writable
	err = testSpoolDirs(s)
	if err != nil {
		log.Printf("[ ] Spool: spool directories not present, spooling may fail: %s\n", err)
	} else {
		log.Println("[X] Spool: spool directories present")
	}

	// start our msg flusher
	go startMsgSpoolFlusher(s)

	// wire up our index page
	s.router.HandleFunc("/", s.handleIndex).Name("Index")

	// register each of our handlers
	for _, handler := range handlers {
		err := handler.Initialize(s)
		if err != nil {
			log.Fatal(err)
		}
	}

	// build a map of the routes we have installed
	var help bytes.Buffer
	s.chanRouter.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		t, err := route.GetPathTemplate()
		if err != nil {
			return err
		}
		help.WriteString(fmt.Sprintf("% 24s: %s\n", route.GetName(), t))
		return nil
	})
	s.routeHelp = help.String()

	// configure timeouts on our server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// and start serving HTTP
	go func() {
		s.waitGroup.Add(1)
		defer s.waitGroup.Done()
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: %s", err)
		}
	}()

	log.Printf("[X] Server: listening on port %d\n", s.config.Port)
	return nil
}

// Stop stops the server, returning only after all threads have stopped
func (s *server) Stop() {
	log.Println("Stopping courier processes")

	if s.db != nil {
		s.db.Close()
	}

	s.redisPool.Close()

	s.stopped = true
	close(s.stopChan)

	// shut down our HTTP server
	if err := s.httpServer.Shutdown(nil); err != nil {
		log.Printf("ERROR gracefully shutting down server: %s\n", err)
	}

	s.waitGroup.Wait()

	log.Printf("[X] Server: stopped listening\n")
}

func (s *server) QueueMsg(msg *Msg) error {
	return queueMsg(s, msg)
}

func (s *server) UpdateMsgStatus(status *MsgStatusUpdate) error {
	return queueMsgStatus(s, status)
}

func (s *server) GetConfig() *config.Courier {
	return s.config
}

func (s *server) GetChannel(cType ChannelType, cUUID string) (*Channel, error) {
	return ChannelFromUUID(s, cType, cUUID)
}

type server struct {
	db        *sqlx.DB
	redisPool *redis.Pool
	s3Client  *s3.S3

	httpServer *http.Server
	router     *mux.Router
	chanRouter *mux.Router

	awsCreds *credentials.Credentials

	config *config.Courier

	waitGroup *sync.WaitGroup
	stopChan  chan bool
	stopped   bool

	routeHelp string
}

func (s *server) Router() *mux.Router { return s.chanRouter }
func (s *server) RouteHelp() string   { return s.routeHelp }

func (s *server) channelFunctionWrapper(handler ChannelHandler, handlerFunc ChannelActionHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := mux.Vars(r)["uuid"]
		channel, err := s.GetChannel(handler.ChannelType(), uuid)
		if err != nil {
			WriteError(w, err)
			return
		}

		err = handlerFunc(channel, w, r)
		if err != nil {
			WriteError(w, err)
		}
	}
}

func (s *server) AddChannelRoute(handler ChannelHandler, method string, action string, handlerFunc ChannelActionHandlerFunc) *mux.Route {
	path := fmt.Sprintf("/%s/{uuid:[a-zA-Z0-9-]{36}}/%s/", strings.ToLower(string(handler.ChannelType())), action)
	route := s.chanRouter.HandleFunc(path, s.channelFunctionWrapper(handler, handlerFunc))
	route.Methods(method)
	route.Name(fmt.Sprintf("%s %s", handler.ChannelName(), strings.Title(action)))
	return route
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// test redis
	rc := s.redisPool.Get()
	_, redisErr := rc.Do("PING")
	defer rc.Close()

	// test our db
	_, dbErr := s.db.Exec("SELECT 1")

	if redisErr == nil && dbErr == nil {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	var buf bytes.Buffer
	buf.WriteString("<title>courier</title><body><pre>\n")
	buf.WriteString(splash)

	if redisErr != nil {
		buf.WriteString(fmt.Sprintf("\n% 16s: %v", "redis err", redisErr))
	}
	if dbErr != nil {
		buf.WriteString(fmt.Sprintf("\n% 16s: %v", "db err", dbErr))
	}

	buf.WriteString("\n\n")
	buf.WriteString(s.RouteHelp())
	buf.WriteString("</pre></body>")
	w.Write(buf.Bytes())
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler ChannelHandler) {
	if handlers == nil {
		handlers = make(map[ChannelType]ChannelHandler)
	}
	handlers[handler.ChannelType()] = handler
}

var handlers map[ChannelType]ChannelHandler
var splash = `
 ____________                   _____             
   ___  ____/_________  ___________(_)____________
    _  /  __  __ \  / / /_  ___/_  /_  _ \_  ___/
    / /__  / /_/ / /_/ /_  /   _  / /  __/  /    
    \____/ \____/\__,_/ /_/    /_/  \___//_/ v0.1                                              
`
