package courier

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/gocommon/httpx"
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

	// GetChannelByAddress returns the channel with the passed in type and address
	GetChannelByAddress(context.Context, ChannelType, ChannelAddress) (Channel, error)

	// GetContact returns (or creates) the contact for the passed in channel and URN
	GetContact(context.Context, Channel, urns.URN, map[string]string, string, bool, *ChannelLog) (Contact, error)

	// AddURNtoContact adds a URN to the passed in contact
	AddURNtoContact(context context.Context, channel Channel, contact Contact, urn urns.URN, authTokens map[string]string) (urns.URN, error)

	// RemoveURNFromcontact removes a URN from the passed in contact
	RemoveURNfromContact(context context.Context, channel Channel, contact Contact, urn urns.URN) (urns.URN, error)

	// DeleteMsgByExternalID deletes a message that has been deleted on the channel side
	DeleteMsgByExternalID(ctx context.Context, channel Channel, externalID string) error

	// NewIncomingMsg creates a new message from the given params
	NewIncomingMsg(context.Context, Channel, urns.URN, string, string, *ChannelLog) MsgIn

	// WriteMsg writes the passed in message to our backend
	WriteMsg(context.Context, MsgIn, *ChannelLog) error

	// NewStatusUpdate creates a new status update for the given message id
	NewStatusUpdate(Channel, MsgID, MsgStatus, *ChannelLog) StatusUpdate

	// NewStatusUpdateByExternalID creates a new status update for the given external id
	NewStatusUpdateByExternalID(Channel, string, MsgStatus, *ChannelLog) StatusUpdate

	// WriteStatusUpdate writes the passed in status update to our backend
	WriteStatusUpdate(context.Context, StatusUpdate) error

	// NewChannelEvent creates a new channel event for the given channel and event type
	NewChannelEvent(Channel, ChannelEventType, urns.URN, *ChannelLog) ChannelEvent

	// WriteChannelEvent writes the passed in channel event returning any error
	WriteChannelEvent(context.Context, ChannelEvent, *ChannelLog) error

	// WriteChannelLog writes the passed in channel log to our backend
	WriteChannelLog(context.Context, *ChannelLog) error

	// PopNextOutgoingMsg returns the next message that needs to be sent, callers should call OnSendComplete with the
	// returned message when they have dealt with the message (regardless of whether it was sent or not)
	PopNextOutgoingMsg(context.Context) (MsgOut, error)

	// WasMsgSent returns whether the backend thinks the passed in message was already sent. This can be used in cases where
	// a backend wants to implement a failsafe against double sending messages (say if they were double queued)
	WasMsgSent(context.Context, MsgID) (bool, error)

	// ClearMsgSent clears any internal status that a message was previously sent. This can be used in the case where
	// a message is being forced in being resent by a user
	ClearMsgSent(context.Context, MsgID) error

	// OnSendComplete is called when the sender has finished trying to send a message
	OnSendComplete(context.Context, MsgOut, StatusUpdate, *ChannelLog)

	// OnReceiveComplete is called when the server has finished handling an incoming request
	OnReceiveComplete(context.Context, Channel, []Event, *ChannelLog)

	// SaveAttachment saves an attachment to backend storage
	SaveAttachment(context.Context, Channel, string, []byte, string) (string, error)

	// ResolveMedia resolves an outgoing attachment URL to a media object
	ResolveMedia(context.Context, string) (Media, error)

	// HttpClient returns an HTTP client for making external requests
	HttpClient(bool) *http.Client
	HttpAccess() *httpx.AccessConfig

	// Health returns a string describing any health problems the backend has, or empty string if all is well
	Health() string

	// Status returns a string describing the current status, this can detail queue sizes or other attributes
	Status() string

	// RedisPool returns the redisPool for this backend
	RedisPool() *redis.Pool
}

// Media is a resolved media object that can be used as a message attachment
type Media interface {
	Name() string
	ContentType() string
	URL() string
	Size() int
	Width() int
	Height() int
	Duration() int
	Alternates() []Media
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
