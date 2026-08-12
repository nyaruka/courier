package channels

import (
	"context"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/core/events"
)

// Event is our interface for the types of things a HandleFunc can return.
type Event interface {
	EventUUID() uuids.UUID
}

// HandleFunc is the interface handlers must satisfy to handle incoming requests.
// The server takes care of looking up the channel by UUID before passing it to this function.
// Errors in format of the request or by the caller should be handled and logged internally. Errors in
// execution or in courier itself should be passed back.
type HandleFunc func(context.Context, *models.Channel, http.ResponseWriter, *http.Request, *models.ChannelLog) ([]Event, error)

// Handler is the interface all channel handlers must satisfy
type Handler interface {
	// SetRuntime is called before Initialize to give the handler the runtime it should use. Handlers embedding
	// handlers.BaseHandler get it from there rather than implementing it themselves.
	SetRuntime(*runtime.Runtime)

	Initialize(*Routes) error
	Runtime() *runtime.Runtime
	ChannelType() models.ChannelType
	ChannelName() string
	UseChannelRouteUUID() bool
	RedactValues(*models.Channel) []string
	GetChannel(context.Context, *http.Request) (*models.Channel, error)
	Send(context.Context, *models.MsgOut, *SendResult, *models.ChannelLog) error

	// SendableEvents returns the engine event types (e.g. typing_started) that can be sent to the
	// given channel's platform, mapped to how often each should be resent to sustain its effect (zero if
	// it never needs resending). Support can vary between channels of the same type, e.g. by config.
	SendableEvents(*models.Channel) map[string]time.Duration
	SendEvent(context.Context, *models.Channel, events.Event, *models.ChannelLog) error

	WriteStatusSuccessResponse(context.Context, http.ResponseWriter, []*models.StatusUpdate) error
	WriteMsgSuccessResponse(context.Context, http.ResponseWriter, []*models.MsgIn) error
	WriteRequestError(context.Context, http.ResponseWriter, error) error
	WriteRequestIgnored(context.Context, http.ResponseWriter, string) error
}

// AttachmentRequestBuilder is the interface handlers which can allow a custom way to download attachment media for messages should satisfy
type AttachmentRequestBuilder interface {
	BuildAttachmentRequest(context.Context, *models.Channel, string, *models.ChannelLog) (*http.Request, error)
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler Handler) {
	registeredHandlers[handler.ChannelType()] = handler

	// handlers which can describe URNs are registered with the models package so contact creation can use them
	if describer, ok := handler.(models.URNDescriber); ok {
		models.RegisterURNDescriber(handler.ChannelType(), describer)
	}
}

// GetHandler returns the handler for the passed in channel type, or nil if not found
func GetHandler(ct models.ChannelType) Handler {
	return registeredHandlers[ct]
}

// RegisteredHandlers returns all the handlers compiled into this build, for the server to initialize
func RegisteredHandlers() []Handler {
	hs := make([]Handler, 0, len(registeredHandlers))
	for _, h := range registeredHandlers {
		hs = append(hs, h)
	}
	return hs
}

// ActivateHandler marks a handler as one this instance is serving, i.e. it was included by config and initialized
func ActivateHandler(handler Handler) {
	activeHandlers[handler.ChannelType()] = handler
}

// GetActiveHandler returns the handler this instance is serving for the given channel type, or nil if that channel
// type isn't being served - which is how sending fails fast for a channel this instance doesn't handle.
func GetActiveHandler(ct models.ChannelType) Handler {
	return activeHandlers[ct]
}

var registeredHandlers = make(map[models.ChannelType]Handler)
var activeHandlers = make(map[models.ChannelType]Handler)

// Route is an HTTP route a channel handler serves, added during its initialization
type Route struct {
	Handler Handler
	Method  string
	Action  string
	LogType svclogs.Type
	Func    HandleFunc
}

// Routes is what a channel handler is initialized with, and collects the routes it wants to serve so that the web
// server can mount them - which is what keeps the handler contract free of any dependency on the server itself.
type Routes struct {
	routes []*Route
}

// NewRoutes creates an empty set of routes for a handler to add to
func NewRoutes() *Routes {
	return &Routes{}
}

// Add adds a route which the handler wants to serve
func (r *Routes) Add(handler Handler, method string, action string, logType svclogs.Type, handlerFunc HandleFunc) {
	r.routes = append(r.routes, &Route{Handler: handler, Method: method, Action: action, LogType: logType, Func: handlerFunc})
}

// All returns every route added so far
func (r *Routes) All() []*Route { return r.routes }
