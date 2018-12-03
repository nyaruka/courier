package courier

import (
	"errors"
	"strings"

	null "gopkg.in/guregu/null.v3"

	uuid "github.com/satori/go.uuid"
)

const (
	// ConfigAPIKey is a constant key for channel configs
	ConfigAPIKey = "api_key"

	// ConfigAuthToken is a constant key for channel configs
	ConfigAuthToken = "auth_token"

	// ConfigBaseURL is a constant key for channel configs
	ConfigBaseURL = "base_url"

	// ConfigCallbackDomain is the domain that should be used for this channel when registering callbacks
	ConfigCallbackDomain = "callback_domain"

	// ConfigContentType is a constant key for channel configs
	ConfigContentType = "content_type"

	// ConfigContentTypeCharset is a constant key for channel configs
	ConfigContentTypeCharset = "content_type_charset"

	// ConfigMaxLength is the maximum size of a message in characters
	ConfigMaxLength = "max_length"

	// ConfigPassword is a constant key for channel configs
	ConfigPassword = "password"

	// ConfigSecret is the secret used for signing commands by the channel
	ConfigSecret = "secret"

	// ConfigSendAuthorization is a constant key for channel configs
	ConfigSendAuthorization = "send_authorization"

	// ConfigSendBody is a constant key for channel configs
	ConfigSendBody = "body"

	// ConfigSendMethod is a constant key for channel configs
	ConfigSendMethod = "method"

	// ConfigSendURL is a constant key for channel configs
	ConfigSendURL = "send_url"

	// ConfigUsername is a constant key for channel configs
	ConfigUsername = "username"
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
	Name() string
	ChannelType() ChannelType
	Schemes() []string
	Country() string
	Address() string

	// CallbackDomain returns the domain that should be used for any callbacks the channel registers
	CallbackDomain(fallbackDomain string) string

	ConfigForKey(key string, defaultValue interface{}) interface{}
	StringConfigForKey(key string, defaultValue string) string
	IntConfigForKey(key string, defaultValue int) int
	OrgConfigForKey(key string, defaultValue interface{}) interface{}
}
