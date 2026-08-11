package courier

import (
	"context"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/goflow/core/events"
)

// ChannelHandleFunc is the interface ChannelHandlers must satisfy to handle incoming requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
// Errors in format of the request or by the caller should be handled and logged internally. Errors in
// execution or in courier itself should be passed back.
type ChannelHandleFunc func(context.Context, *models.Channel, http.ResponseWriter, *http.Request, *models.ChannelLog) ([]Event, error)

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(*Server) error
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
func RegisterHandler(handler ChannelHandler) {
	registeredHandlers[handler.ChannelType()] = handler

	// handlers which can describe URNs are registered with the models package so contact creation can use them
	if describer, ok := handler.(models.URNDescriber); ok {
		models.RegisterURNDescriber(handler.ChannelType(), describer)
	}
}

// GetHandler returns the handler for the passed in channel type, or nil if not found
func GetHandler(ct models.ChannelType) ChannelHandler {
	return registeredHandlers[ct]
}

var registeredHandlers = make(map[models.ChannelType]ChannelHandler)
var activeHandlers = make(map[models.ChannelType]ChannelHandler)
