package courier

import (
	"fmt"
	"strings"

	"github.com/nyaruka/courier/config"
)

// BackendConstructorFunc defines a function to create a particular backend type
type BackendConstructorFunc func(*config.Courier) Backend

// Backend represents the part of Courier that deals with looking up and writing channels and results
type Backend interface {
	Start() error
	Stop() error

	GetChannel(ChannelType, ChannelUUID) (Channel, error)
	WriteMsg(*Msg) error
	WriteMsgStatus(*MsgStatusUpdate) error
	WriteChannelLogs([]*ChannelLog) error

	PopNextOutgoingMsg() (*Msg, error)
	MarkOutgoingMsgComplete(*Msg)

	Health() string
}

// NewBackend creates the type of backend passed in
func NewBackend(config *config.Courier) (Backend, error) {
	backendFunc, found := registeredBackends[strings.ToLower(config.Backend)]
	if !found {
		return nil, fmt.Errorf("no such backend type: '%s'", config.Backend)
	}
	return backendFunc(config), nil
}

// RegisterBackend adds a new backend, called by individual backends in their init() func
func RegisterBackend(backendType string, constructorFunc BackendConstructorFunc) {
	registeredBackends[strings.ToLower(backendType)] = constructorFunc
}

var registeredBackends = make(map[string]BackendConstructorFunc)
