package courier

import (
	"net/http"
)

// ChannelReceiveMsgFunc is the interface ChannelHandler functions must satisfy to handle incoming msgs
// The Server will take care of looking up the channel by UUID before passing it to this function.
type ChannelReceiveMsgFunc func(Channel, http.ResponseWriter, *http.Request) ([]Msg, error)

// ChannelUpdateStatusFunc is the interface ChannelHandler functions must satisfy to handle incoming
// status requests. The Server will take care of looking up the channel by UUID before passing it to this function.
type ChannelUpdateStatusFunc func(Channel, http.ResponseWriter, *http.Request) ([]MsgStatus, error)

// ChannelActionHandlerFunc is the interface ChannelHandler functions must satisfy to handle other types
// of requests. These generic handlers should only be used if they are not dealing with receiving messages
// or status updates.
//
// The Server will take care of looking up the channel by UUID before passing it to this function.
type ChannelActionHandlerFunc func(Channel, http.ResponseWriter, *http.Request) error

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
