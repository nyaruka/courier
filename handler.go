package courier

import (
	"context"
	"net/http"

	"github.com/nyaruka/gocommon/urns"
)

// Event is our interface for the types of things a ChannelHandleFunc can return.
type Event interface {
	EventID() int64
}

// ChannelHandleFunc is the interface ChannelHandlers must satisfy to handle incoming requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
// Errors in format of the request or by the caller should be handled and logged internally. Errors in
// execution or in courier itself should be passed back.
type ChannelHandleFunc func(context.Context, Channel, http.ResponseWriter, *http.Request, *ChannelLog) ([]Event, error)

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(Server) error
	Server() Server
	ChannelType() ChannelType
	ChannelName() string
	UseChannelRouteUUID() bool
	RedactValues(Channel) []string
	GetChannel(context.Context, *http.Request) (Channel, error)
	Send(context.Context, MsgOut, *ChannelLog) (StatusUpdate, error)

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
	BuildAttachmentRequest(context.Context, Backend, Channel, string, *ChannelLog) (*http.Request, error)
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler ChannelHandler) {
	registeredHandlers[handler.ChannelType()] = handler
}

// GetHandler returns the handler for the passed in channel type, or nil if not found
func GetHandler(ct ChannelType) ChannelHandler {
	return registeredHandlers[ct]
}

var registeredHandlers = make(map[ChannelType]ChannelHandler)
var activeHandlers = make(map[ChannelType]ChannelHandler)
