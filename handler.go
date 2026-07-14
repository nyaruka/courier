package courier

import (
	"context"
	"net/http"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/urns"
)

// ChannelHandleFunc is the interface ChannelHandlers must satisfy to handle incoming requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
// Errors in format of the request or by the caller should be handled and logged internally. Errors in
// execution or in courier itself should be passed back.
type ChannelHandleFunc func(context.Context, Channel, http.ResponseWriter, *http.Request, *ChannelLog) ([]Event, error)

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(*Server) error
	Runtime() *runtime.Runtime
	Backend() Backend
	ChannelType() models.ChannelType
	ChannelName() string
	UseChannelRouteUUID() bool
	RedactValues(Channel) []string
	GetChannel(context.Context, *http.Request) (Channel, error)
	Send(context.Context, MsgOut, *SendResult, *ChannelLog) error

	// ChatActions returns the chat actions that can be sent on the given channel, mapped to how often each
	// should be resent to sustain it (zero if it never needs resending). Support can vary between channels
	// of the same type, e.g. by config.
	ChatActions(Channel) map[ChatAction]time.Duration
	SendChatAction(context.Context, Channel, ChatAction, urns.URN, *ChannelLog) error

	WriteStatusSuccessResponse(context.Context, http.ResponseWriter, []StatusUpdate) error
	WriteMsgSuccessResponse(context.Context, http.ResponseWriter, []MsgIn) error
	WriteRequestError(context.Context, http.ResponseWriter, error) error
	WriteRequestIgnored(context.Context, http.ResponseWriter, string) error
}

// URNDescriber is the interface handlers which can look up URN metadata for new contacts should satisfy.
type URNDescriber interface {
	DescribeURN(context.Context, Channel, urns.URN, *ChannelLog) (map[string]string, error)
}

// AttachmentRequestBuilder is the interface handlers which can allow a custom way to download attachment media for messages should satisfy
type AttachmentRequestBuilder interface {
	BuildAttachmentRequest(context.Context, Channel, string, *ChannelLog) (*http.Request, error)
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler ChannelHandler) {
	registeredHandlers[handler.ChannelType()] = handler
}

// GetHandler returns the handler for the passed in channel type, or nil if not found
func GetHandler(ct models.ChannelType) ChannelHandler {
	return registeredHandlers[ct]
}

var registeredHandlers = make(map[models.ChannelType]ChannelHandler)
var activeHandlers = make(map[models.ChannelType]ChannelHandler)
