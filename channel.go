package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
type ChannelID int64

// NewChannelID creates a new ChannelID for the passed in int64
func NewChannelID(id int64) ChannelID {
	return ChannelID(id)
}

// UnmarshalText satisfies text unmarshalling so ids can be decoded from forms
func (i *ChannelID) UnmarshalText(text []byte) (err error) {
	id, err := strconv.ParseInt(string(text), 10, 64)
	*i = ChannelID(id)
	if err != nil {
		return err
	}
	return err
}

// UnmarshalJSON satisfies json unmarshalling so ids can be decoded from JSON
func (i *ChannelID) UnmarshalJSON(bytes []byte) (err error) {
	var id int64
	err = json.Unmarshal(bytes, &id)
	*i = ChannelID(id)
	return err
}

// MarshalJSON satisfies json marshalling so ids can be encoded to JSON
func (i *ChannelID) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(*i))
}

// String satisfies the Stringer interface
func (i *ChannelID) String() string {
	return fmt.Sprintf("%d", i)
}

// NilChannelID is our nil value for ChannelIDs
var NilChannelID = ChannelID(0)

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
}
