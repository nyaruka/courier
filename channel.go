package courier

import (
	"errors"
	"strings"

	null "gopkg.in/guregu/null.v3"

	uuid "github.com/satori/go.uuid"
)

const (
	// ConfigAuthToken is a constant key for channel configs
	ConfigAuthToken = "auth_token"

	// ConfigUsername is a constant key for channel configs
	ConfigUsername = "username"

	// ConfigPassword is a constant key for channel configs
	ConfigPassword = "password"

	// ConfigAPIKey is a constant key for channel configs
	ConfigAPIKey = "api_key"

	// ConfigSendURL is a constant key for channel configs
	ConfigSendURL = "send_url"

	// ConfigSendBody is a constant key for channel configs
	ConfigSendBody = "send_body"

	// ConfigSendMethod is a constant key for channel configs
	ConfigSendMethod = "send_method"

	// ConfigContentType is a constant key for channel configs
	ConfigContentType = "content_type"
)

// ChannelType is our typing of the two char channel types
type ChannelType string

// AnyChannelType is our empty channel type used when doing lookups without channel type assertions
var AnyChannelType = ChannelType("")

func (ct ChannelType) String() string {
	return string(ct)
}

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

// ChannelID is our SQL type for a channel's id
type ChannelID struct {
	null.Int
}

// NewChannelID creates a new ChannelID for the passed in int64
func NewChannelID(id int64) ChannelID {
	return ChannelID{null.NewInt(id, true)}
}

// NilChannelID is our nil value for ChannelIDs
var NilChannelID = ChannelID{null.NewInt(0, false)}

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
	Schemes() []string
	Country() string
	Address() string
	ConfigForKey(key string, defaultValue interface{}) interface{}
	StringConfigForKey(key string, defaultValue string) string

	OrgConfigForKey(key string, defaultValue interface{}) interface{}
}
