package courier

import (
	"net/http"
)

// Event is our interface for the types of things a ChannelHandleFunc can return.
type Event interface {
	EventID() int64
}

// ChannelHandleFunc is the interface ChannelHandlers must satisfy to handle incoming requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
// Errors in format of the request or by the caller should be handled and logged internally. Errors in
// execution or in courier itself should be passed back.
type ChannelHandleFunc func(Channel, http.ResponseWriter, *http.Request) ([]Event, error)

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(Server) error
	ChannelType() ChannelType
	ChannelName() string
	SendMsg(Msg) (MsgStatus, error)
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler ChannelHandler) {
	registeredHandlers[handler.ChannelType()] = handler
}

var registeredHandlers = make(map[ChannelType]ChannelHandler)
var activeHandlers = make(map[ChannelType]ChannelHandler)
