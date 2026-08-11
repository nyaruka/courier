package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
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

// Channel is a channel as loaded from the database, along with the config of the org which owns it
type Channel struct {
	OrgID_       OrgID          `db:"org_id"`
	UUID_        ChannelUUID    `db:"uuid"`
	ID_          ChannelID      `db:"id"`
	ChannelType_ ChannelType    `db:"channel_type"`
	Schemes_     pq.StringArray `db:"schemes"`
	Name_        sql.NullString `db:"name"`
	Address_     sql.NullString `db:"address"`
	Country_     sql.NullString `db:"country"`
	Config_      null.Map[any]  `db:"config"`
	Role_        string         `db:"role"`

	OrgConfig_ null.Map[any] `db:"org_config"`
	OrgIsAnon_ bool          `db:"org_is_anon"`
}

func (c *Channel) ID() ChannelID            { return c.ID_ }
func (c *Channel) UUID() ChannelUUID        { return c.UUID_ }
func (c *Channel) OrgID() OrgID             { return c.OrgID_ }
func (c *Channel) OrgIsAnon() bool          { return c.OrgIsAnon_ }
func (c *Channel) ChannelType() ChannelType { return c.ChannelType_ }
func (c *Channel) Name() string             { return c.Name_.String }
func (c *Channel) Schemes() []string        { return []string(c.Schemes_) }
func (c *Channel) Address() string          { return c.Address_.String }

// ChannelAddress returns the address of this channel
func (c *Channel) ChannelAddress() ChannelAddress {
	if !c.Address_.Valid {
		return NilChannelAddress
	}

	return ChannelAddress(c.Address_.String)
}

// Country returns the country code for this channel if any
func (c *Channel) Country() i18n.Country { return i18n.Country(c.Country_.String) }

// IsScheme returns whether this channel serves only the passed in scheme
func (c *Channel) IsScheme(scheme *urns.Scheme) bool {
	return len(c.Schemes_) == 1 && c.Schemes_[0] == scheme.Prefix
}

// Roles returns the roles of this channel
func (c *Channel) Roles() []ChannelRole {
	roles := []ChannelRole{}
	for _, char := range strings.Split(c.Role_, "") {
		roles = append(roles, ChannelRole(char))
	}
	return roles
}

// HasRole returns whether the passed in channel supports the passed role
func (c *Channel) HasRole(role ChannelRole) bool {
	for _, r := range c.Roles() {
		if r == role {
			return true
		}
	}
	return false
}

// ConfigForKey returns the config value for the passed in key, or defaultValue if it isn't found
func (c *Channel) ConfigForKey(key string, defaultValue any) any {
	value, found := c.Config_[key]
	if !found {
		return defaultValue
	}
	return value
}

// OrgConfigForKey returns the org config value for the passed in key, or defaultValue if it isn't found
func (c *Channel) OrgConfigForKey(key string, defaultValue any) any {
	value, found := c.OrgConfig_[key]
	if !found {
		return defaultValue
	}
	return value
}

// StringConfigForKey returns the config value for the passed in key, or defaultValue if it isn't found
func (c *Channel) StringConfigForKey(key string, defaultValue string) string {
	val := c.ConfigForKey(key, defaultValue)
	str, isStr := val.(string)
	if !isStr {
		return defaultValue
	}
	return str
}

// BoolConfigForKey returns the config value for the passed in key, or defaultValue if it isn't found
func (c *Channel) BoolConfigForKey(key string, defaultValue bool) bool {
	val := c.ConfigForKey(key, defaultValue)
	b, isBool := val.(bool)
	if !isBool {
		return defaultValue
	}
	return b
}

// IntConfigForKey returns the config value for the passed in key
func (c *Channel) IntConfigForKey(key string, defaultValue int) int {
	val := c.ConfigForKey(key, defaultValue)

	// golang unmarshals number literals in JSON into float64s by default
	f, isFloat := val.(float64)
	if isFloat {
		return int(f)
	}

	str, isStr := val.(string)
	if isStr {
		i, err := strconv.Atoi(str)
		if err == nil {
			return i
		}
	}
	return defaultValue
}

// CallbackDomain is convenience utility to get the callback domain configured for this channel
func (c *Channel) CallbackDomain(fallbackDomain string) string {
	return c.StringConfigForKey(ConfigCallbackDomain, fallbackDomain)
}

const sqlSelectChannelFromUUID = `
SELECT
	c.uuid,
	c.org_id,
	c.id,
	c.channel_type,
	c.name,
	c.schemes,
	c.address,
	c.country,
	c.config,
	c.role,
	o.config AS org_config,
	o.is_anon AS org_is_anon
  FROM channels_channel c
  JOIN orgs_org o ON c.org_id = o.id
 WHERE c.uuid = $1 AND c.is_active = TRUE AND c.org_id IS NOT NULL`

func loadChannelByUUID(ctx context.Context, rt *runtime.Runtime, uuid ChannelUUID) (*Channel, error) {
	channel := &Channel{}
	err := rt.DB.GetContext(ctx, channel, sqlSelectChannelFromUUID, uuid)

	if err == sql.ErrNoRows {
		return nil, ErrChannelNotFound
	}
	return channel, err
}

const sqlSelectChannelFromAddress = `
SELECT
	c.uuid,
	c.org_id,
	c.id,
	c.channel_type,
	c.name,
	c.schemes,
	c.address,
	c.country,
	c.config,
	c.role,
	o.config AS org_config,
	o.is_anon AS org_is_anon
  FROM channels_channel c
  JOIN orgs_org o ON c.org_id = o.id
 WHERE c.address = $1 AND c.is_active = TRUE AND c.org_id IS NOT NULL`

func loadChannelByAddress(ctx context.Context, rt *runtime.Runtime, addr ChannelAddress) (*Channel, error) {
	channel := &Channel{}
	err := rt.DB.GetContext(ctx, channel, sqlSelectChannelFromAddress, addr)

	if err == sql.ErrNoRows {
		return nil, ErrChannelNotFound
	}
	return channel, err
}

// GetChannel returns the channel with the passed in type and UUID. It reads through the channel cache created by
// Start, which is why - unlike the rest of this package - it doesn't take a runtime.
func GetChannel(ctx context.Context, typ ChannelType, uuid ChannelUUID) (*Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	ch, err := channelsByUUID.GetOrFetch(timeout, uuid)
	if err != nil {
		return nil, err
	}

	if typ != AnyChannelType && ch.ChannelType() != typ {
		return nil, ErrChannelWrongType
	}

	return ch, nil
}

// GetChannelByAddress returns the channel with the passed in type and address. Like GetChannel it reads through the
// cache created by Start rather than taking a runtime.
func GetChannelByAddress(ctx context.Context, typ ChannelType, address ChannelAddress) (*Channel, error) {
	timeout, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	ch, err := channelsByAddr.GetOrFetch(timeout, address)
	if err != nil {
		return nil, err
	}

	if typ != AnyChannelType && ch.ChannelType() != typ {
		return nil, ErrChannelWrongType
	}

	return ch, nil
}

// FlushChannelCache clears the channel caches - used in tests after channels are modified in the database. It's a
// no-op if the caches haven't been created, i.e. Start hasn't been called.
func FlushChannelCache() {
	if channelsByUUID != nil {
		channelsByUUID.Clear()
	}
	if channelsByAddr != nil {
		channelsByAddr.Clear()
	}
}
