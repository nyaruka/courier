package courier

import (
	"errors"
	"strings"

	uuid "github.com/satori/go.uuid"
)

const (
	// ConfigAuthToken is our constant key used in channel configs for auth tokens
	ConfigAuthToken = "auth_token"
)

// ChannelType is our typing of the two char channel types
type ChannelType string

// AnyChannelType is our empty channel type used when doing lookups without channel type assertions
var AnyChannelType = ChannelType("")

// ChannelUUID is our typing of a channel's UUID
type ChannelUUID struct {
	uuid.UUID
}

// NilChannelUUID is our nil value for channel UUIDs
var NilChannelUUID = ChannelUUID{uuid.Nil}

// NewChannelUUID creates a new ChannelUUID for the passed in string
func NewChannelUUID(u string) (ChannelUUID, error) {
	channelUUID, err := uuid.FromString(strings.ToLower(u))
	if err != nil {
		return NilChannelUUID, err
	}
	return ChannelUUID{channelUUID}, nil
}

// ErrChannelExpired is returned when our cached channel has outlived it's TTL
var ErrChannelExpired = errors.New("channel expired")

// ErrChannelNotFound is returned when we fail to find a channel in the db
var ErrChannelNotFound = errors.New("channel not found")

// ErrChannelWrongType is returned when we find a channel with the set UUID but with a different type
var ErrChannelWrongType = errors.New("channel type wrong")

//-----------------------------------------------------------------------------
// Channel Interface
//-----------------------------------------------------------------------------

// Channel defines the general interface backend Channel implementations must adhere to
type Channel interface {
	UUID() ChannelUUID
	ChannelType() ChannelType
	Country() string
	Address() string
	ConfigForKey(key string, defaultValue interface{}) interface{}
}
