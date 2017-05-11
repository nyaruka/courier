package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/nyaruka/courier/config"
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
	testConfig := config.Courier{Base_URL: "http://courier.test"}
	channels := make(map[ChannelUUID]*Channel)
	router := mux.NewRouter()
	chanRouter := router.PathPrefix("/c/").Subrouter()
	ts := &MockServer{config: &testConfig, channels: channels, router: router, chanRouter: chanRouter}
	return ts
}

func (ts *MockServer) Router() *mux.Router { return ts.router }

func (ts *MockServer) GetLastQueueMsg() (*Msg, error) {
	if len(ts.queueMsgs) == 0 {
		return nil, ErrMsgNotFound
	}
	return ts.queueMsgs[len(ts.queueMsgs)-1], nil
}

func (ts *MockServer) SetErrorOnQueue(shouldError bool) {
	ts.errorOnQueue = shouldError
}

func (ts *MockServer) QueueMsg(m *Msg) error {
	if ts.errorOnQueue {
		return errors.New("unable to queue message")
	}

	ts.queueMsgs = append(ts.queueMsgs, m)
	return nil
}

func (ts *MockServer) UpdateMsgStatus(status *MsgStatusUpdate) error {
	return nil
}

func (ts *MockServer) SaveMedia(Msg, []byte) (string, error) {
	return "", fmt.Errorf("Save media not implemented on test server")
}

func (ts *MockServer) GetConfig() *config.Courier {
	return ts.config
}

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

func (ts *MockServer) ClearChannels() {
	ts.channels = nil
}

func (ts *MockServer) Start() error { return nil }
func (ts *MockServer) Stop()        {}
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

func NewMockChannel(uuid string, channelType string, address string, country string, config map[string]string) *Channel {
	cUUID, _ := NewChannelUUID(uuid)

	configJSON := ""
	if config != nil {
		configBytes, err := json.Marshal(config)
		if err != nil {
			panic(err)
		}
		configJSON = string(configBytes)
	}

	channel := &Channel{
		UUID:        cUUID,
		ChannelType: ChannelType(channelType),
		Address:     address,
		Country:     country,
		Config:      configJSON,
	}
	channel.parseConfig()
	return channel
}
