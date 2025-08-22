package models

import (
	"database/sql/driver"
	"errors"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
)

// ChannelType is the 1-3 letter code used for channel types in the database
type ChannelType string

// AnyChannelType is our empty channel type used when doing lookups without channel type assertions
const AnyChannelType = ChannelType("")

// ChannelRole is a role that a channel can perform
type ChannelRole string

// different roles that channels can perform
const (
	ChannelRoleSend    ChannelRole = "S"
	ChannelRoleReceive ChannelRole = "R"
	ChannelRoleCall    ChannelRole = "C"
	ChannelRoleAnswer  ChannelRole = "A"
)

// ChannelUUID is our typing of a channel's UUID
type ChannelUUID uuids.UUID

// NilChannelUUID is our nil value for channel UUIDs
var NilChannelUUID = ChannelUUID("")

// ChannelID is our SQL type for a channel's id
type ChannelID null.Int

// NilChannelID represents a nil channel id
const NilChannelID = ChannelID(0)

// NewChannelID creates a new ChannelID for the passed in int64
func NewChannelID(id int64) ChannelID {
	return ChannelID(id)
}

func (i *ChannelID) Scan(value any) error         { return null.ScanInt(value, i) }
func (i ChannelID) Value() (driver.Value, error)  { return null.IntValue(i) }
func (i *ChannelID) UnmarshalJSON(b []byte) error { return null.UnmarshalInt(b, i) }
func (i ChannelID) MarshalJSON() ([]byte, error)  { return null.MarshalInt(i) }

// ChannelAddress is our SQL type for a channel address
type ChannelAddress null.String

// NilChannelAddress represents a nil channel address
const NilChannelAddress = ChannelAddress("")

func (a ChannelAddress) String() string {
	return string(a)
}

type LogPolicy string

const (
	LogPolicyNone   = "N"
	LogPolicyErrors = "E"
	LogPolicyAll    = "A"
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

	// ConfigUseNational is a constant key for channel configs
	ConfigUseNational = "use_national"

	// ConfigSendHeaders is a constant key for channel configs
	ConfigSendHeaders = "headers"
)

// ErrChannelExpired is returned when our cached channel has outlived it's TTL
var ErrChannelExpired = errors.New("channel expired")

// ErrChannelNotFound is returned when we fail to find a channel in the db
var ErrChannelNotFound = errors.New("channel not found")

// ErrChannelWrongType is returned when we find a channel with the set UUID but with a different type
var ErrChannelWrongType = errors.New("channel type wrong")
