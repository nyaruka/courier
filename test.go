package courier

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier/config"
	"github.com/nyaruka/courier/utils"
)

var testDatabaseURL = "postgres://courier@localhost/courier_test?sslmode=disable"
var testRedisURL = "redis://localhost:6379/10"
var testConfig = config.Courier{
	DB:    testDatabaseURL,
	Redis: testRedisURL,
}

//-----------------------------------------------------------------------------
// Mock server implementation
//-----------------------------------------------------------------------------

// MockServer is a mocked version of server which doesn't require a real database or cache
type MockServer struct {
	config       *config.Courier
	channels     map[ChannelUUID]*Channel
	queueMsgs    []*Msg
	errorOnQueue bool

	router     *mux.Router
	chanRouter *mux.Router
}

// NewMockServer creates a new mock server
func NewMockServer() *MockServer {
	testConfig := config.Courier{BaseURL: "http://courier.test"}
	channels := make(map[ChannelUUID]*Channel)
	router := mux.NewRouter()
	chanRouter := router.PathPrefix("/c/").Subrouter()
	ts := &MockServer{config: &testConfig, channels: channels, router: router, chanRouter: chanRouter}
	return ts
}

// Router returns the Gorilla router our server
func (ts *MockServer) Router() *mux.Router { return ts.router }

// GetLastQueueMsg returns the last message queued to the server
func (ts *MockServer) GetLastQueueMsg() (*Msg, error) {
	if len(ts.queueMsgs) == 0 {
		return nil, ErrMsgNotFound
	}
	return ts.queueMsgs[len(ts.queueMsgs)-1], nil
}

// SetErrorOnQueue is a mock method which makes the QueueMsg call throw the passed in error on next call
func (ts *MockServer) SetErrorOnQueue(shouldError bool) {
	ts.errorOnQueue = shouldError
}

// QueueMsg queues the passed in message internally
func (ts *MockServer) QueueMsg(m *Msg) error {
	if ts.errorOnQueue {
		return errors.New("unable to queue message")
	}

	ts.queueMsgs = append(ts.queueMsgs, m)
	return nil
}

// UpdateMsgStatus writes the status update to our queue
func (ts *MockServer) UpdateMsgStatus(status *MsgStatusUpdate) error {
	return nil
}

// GetConfig returns the config for our server
func (ts *MockServer) GetConfig() *config.Courier {
	return ts.config
}

// GetChannel returns
func (ts *MockServer) GetChannel(cType ChannelType, uuid string) (*Channel, error) {
	cUUID, err := NewChannelUUID(uuid)
	if err != nil {
		return nil, err
	}
	channel, found := ts.channels[cUUID]
	if !found {
		return nil, ErrChannelNotFound
	}
	return channel, nil
}

// AddChannel adds a test channel to the test server
func (ts *MockServer) AddChannel(channel *Channel) {
	ts.channels[channel.UUID] = channel
}

// ClearChannels is a utility function on our mock server to clear all added channels
func (ts *MockServer) ClearChannels() {
	ts.channels = nil
}

// Start starts our mock server
func (ts *MockServer) Start() error { return nil }

// Stop stops our mock server
func (ts *MockServer) Stop() {}

// ClearQueueMsgs clears our mock msg queue
func (ts *MockServer) ClearQueueMsgs() {
	ts.queueMsgs = nil
}

func (ts *MockServer) channelFunctionWrapper(handler ChannelHandler, handlerFunc ChannelActionHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := mux.Vars(r)["uuid"]
		channel, err := ts.GetChannel(handler.ChannelType(), uuid)
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

// AddChannelRoute adds the passed in handler to our router
func (ts *MockServer) AddChannelRoute(handler ChannelHandler, method string, action string, handlerFunc ChannelActionHandlerFunc) *mux.Route {
	path := fmt.Sprintf("/%s/{uuid:[a-zA-Z0-9-]{36}}/%s/", strings.ToLower(string(handler.ChannelType())), action)
	route := ts.chanRouter.HandleFunc(path, ts.channelFunctionWrapper(handler, handlerFunc))
	route.Methods(method)
	route.Name(fmt.Sprintf("%s %s", handler.ChannelName(), strings.Title(action)))
	return route
}

//-----------------------------------------------------------------------------
// Mock channel implementation
//-----------------------------------------------------------------------------

// NewMockChannel creates a new mock channel for the passed in type, address, country and config
func NewMockChannel(uuid string, channelType string, address string, country string, config map[string]string) *Channel {
	cUUID, _ := NewChannelUUID(uuid)

	channel := &Channel{
		UUID:        cUUID,
		ChannelType: ChannelType(channelType),
		Address:     sql.NullString{String: address, Valid: true},
		Country:     sql.NullString{String: country, Valid: true},
		Config:      utils.NullDict{Dict: config, Valid: true},
	}
	return channel
}
