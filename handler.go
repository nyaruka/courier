package courier

import (
	"net/http"
)

// ChannelActionHandlerFunc is the interface ChannelHandler functions must satisfy to handle various requests.
// The Server will take care of looking up the channel by UUID before passing it to this function.
type ChannelActionHandlerFunc func(Channel, http.ResponseWriter, *http.Request) error

// ChannelHandler is the interface all handlers must satisfy
type ChannelHandler interface {
	Initialize(Server) error
	ChannelType() ChannelType
	ChannelName() string
}

// RegisterHandler adds a new handler for a channel type, this is called by individual handlers when they are initialized
func RegisterHandler(handler ChannelHandler) {
	registeredHandlers[handler.ChannelType()] = handler
}

var registeredHandlers = make(map[ChannelType]ChannelHandler)
var activeHandlers = make(map[ChannelType]ChannelHandler)
