package courier

import (
	"context"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// Backend represents the part of Courier that deals with looking up and writing channels and results
type Backend interface {
	// Start starts the backend and opens any db connections it needs
	Start() error

	// Stop stops any backend processes
	Stop() error

	// GetChannel returns the channel with the passed in type and UUID
	GetChannel(context.Context, models.ChannelType, models.ChannelUUID) (*models.Channel, error)

	// GetChannelByAddress returns the channel with the passed in type and address
	GetChannelByAddress(context.Context, models.ChannelType, models.ChannelAddress) (*models.Channel, error)

	// GetContact returns (or creates) the contact for the passed in channel and URN
	GetContact(context.Context, *models.Channel, urns.URN, map[string]string, string, bool, *models.ChannelLog) (*models.Contact, error)

	// DeleteMsgByExternalID deletes a message that has been deleted on the channel side
	DeleteMsgByExternalID(ctx context.Context, channel *models.Channel, externalID string) error

	// NewIncomingMsg creates a new message from the given params
	NewIncomingMsg(context.Context, *models.Channel, urns.URN, string, string, *models.ChannelLog) MsgIn

	// WriteMsg writes the passed in message to our backend
	WriteMsg(context.Context, MsgIn, *models.ChannelLog) error

	// NewStatusUpdate creates a new status update for the given message id
	NewStatusUpdate(*models.Channel, models.MsgUUID, models.MsgStatus, *models.ChannelLog) *models.StatusUpdate

	// NewStatusUpdateByExternalID creates a new status update for the given external id
	NewStatusUpdateByExternalID(*models.Channel, string, models.MsgStatus, *models.ChannelLog) *models.StatusUpdate

	// WriteStatusUpdate writes the passed in status update to our backend
	WriteStatusUpdate(context.Context, *models.StatusUpdate) error

	// NewChannelEvent creates a new channel event for the given channel and event type
	NewChannelEvent(*models.Channel, models.ChannelEventType, urns.URN, *models.ChannelLog) ChannelEvent

	// WriteChannelEvent writes the passed in channel event returning any error
	WriteChannelEvent(context.Context, ChannelEvent, *models.ChannelLog) error

	// WriteChannelLog writes the passed in channel log to our backend
	WriteChannelLog(context.Context, *models.ChannelLog) error

	// PopNextOutgoingMsg returns the next message that needs to be sent, callers should call OnSendComplete with the
	// returned message when they have dealt with the message (regardless of whether it was sent or not)
	PopNextOutgoingMsg(context.Context) (MsgOut, error)

	// WasMsgSent returns whether the backend thinks the passed in message was already sent. This can be used in cases where
	// a backend wants to implement a failsafe against double sending messages (say if they were double queued)
	WasMsgSent(context.Context, models.MsgUUID) (bool, error)

	// ClearMsgSent clears any internal status that a message was previously sent. This can be used in the case where
	// a message is being forced in being resent by a user
	ClearMsgSent(context.Context, models.MsgUUID) error

	// OnSendComplete is called when the sender has finished trying to send a message
	OnSendComplete(context.Context, MsgOut, *models.StatusUpdate, *SendResult, *models.ChannelLog)

	// OnReceiveComplete is called when the server has finished handling an incoming request
	OnReceiveComplete(context.Context, *models.Channel, []Event, *models.ChannelLog)

	// SaveAttachment saves an attachment to backend storage
	SaveAttachment(context.Context, *models.Channel, string, []byte, string) (string, error)

	// ResolveMedia resolves an outgoing attachment URL to a media object
	ResolveMedia(context.Context, string) (*models.Media, error)

	// RedisPool returns the redisPool for this backend
	RedisPool() *redis.Pool
}
