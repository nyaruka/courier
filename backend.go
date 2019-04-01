package courier

import (
	"context"
	"fmt"
	"strings"

	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/gocommon/urns"
)

// BackendConstructorFunc defines a function to create a particular backend type
type BackendConstructorFunc func(*Config) Backend

// Backend represents the part of Courier that deals with looking up and writing channels and results
type Backend interface {
	// Start starts the backend and opens any db connections it needs
	Start() error

	// Stop stops any backend processes
	Stop() error

	// Cleanup closes any active connections to databases
	Cleanup() error

	// GetChannel returns the channel with the passed in type and UUID
	GetChannel(context.Context, ChannelType, ChannelUUID) (Channel, error)

	// GetContact returns (or creates) the contact for the passed in channel and URN
	GetContact(context context.Context, channel Channel, urn urns.URN, auth string, name string) (Contact, error)

	// AddURNtoContact adds a URN to the passed in contact
	AddURNtoContact(context context.Context, channel Channel, contact Contact, urn urns.URN) (urns.URN, error)

	// RemoveURNFromcontact removes a URN from the passed in contact
	RemoveURNfromContact(context context.Context, channel Channel, contact Contact, urn urns.URN) (urns.URN, error)

	// NewIncomingMsg creates a new message from the given params
	NewIncomingMsg(channel Channel, urn urns.URN, text string) Msg

	// WriteMsg writes the passed in message to our backend
	WriteMsg(context.Context, Msg) error

	// NewMsgStatusForID creates a new Status object for the given message id
	NewMsgStatusForID(Channel, MsgID, MsgStatusValue) MsgStatus

	// NewMsgStatusForExternalID creates a new Status object for the given external id
	NewMsgStatusForExternalID(Channel, string, MsgStatusValue) MsgStatus

	// WriteMsgStatus writes the passed in status update to our backend
	WriteMsgStatus(context.Context, MsgStatus) error

	// NewChannelEvent creates a new channel event for the given channel and event type
	NewChannelEvent(Channel, ChannelEventType, urns.URN) ChannelEvent

	// WriteChannelEvent writes the passed in channel even returning any error
	WriteChannelEvent(context.Context, ChannelEvent) error

	// WriteChannelLogs writes the passed in channel logs to our backend
	WriteChannelLogs(context.Context, []*ChannelLog) error

	// PopNextOutgoingMsg returns the next message that needs to be sent, callers should call MarkOutgoingMsgComplete with the
	// returned message when they have dealt with the message (regardless of whether it was sent or not)
	PopNextOutgoingMsg(context.Context) (Msg, error)

	// WasMsgSent returns whether the backend thinks the passed in message was already sent. This can be used in cases where
	// a backend wants to implement a failsafe against double sending messages (say if they were double queued)
	WasMsgSent(context.Context, Msg) (bool, error)

	// MarkOutgoingMsgComplete marks the passed in message as having been processed. Note this should be called even in the case
	// of errors during sending as it will manage the number of active workers per channel. The optional status parameter can be
	// used to determine any sort of deduping of msg sends
	MarkOutgoingMsgComplete(context.Context, Msg, MsgStatus)

	// Check if external ID has been seen in a period
	CheckExternalIDSeen(Msg) Msg

	// Mark a external ID as seen for a period
	WriteExternalIDSeen(Msg)

	// Health returns a string describing any health problems the backend has, or empty string if all is well
	Health() string

	// Status returns a string describing the current status, this can detail queue sizes or other attributes
	Status() string

	// Heartbeat is called every minute, it can be used by backends to log status to a dashboard such as librato
	Heartbeat() error

	// RedisPool returns the redisPool for this backend
	RedisPool() *redis.Pool
}

// NewBackend creates the type of backend passed in
func NewBackend(config *Config) (Backend, error) {
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
