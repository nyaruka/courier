package rapidpro

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/null/v3"
)

type LogPolicy string

const (
	LogPolicyNone   = "N"
	LogPolicyErrors = "E"
	LogPolicyAll    = "A"
)

// Channel is the RapidPro specific concrete type satisfying the courier.Channel interface
type Channel struct {
	OrgID_       OrgID               `db:"org_id"`
	UUID_        courier.ChannelUUID `db:"uuid"`
	ID_          courier.ChannelID   `db:"id"`
	ChannelType_ courier.ChannelType `db:"channel_type"`
	Schemes_     pq.StringArray      `db:"schemes"`
	Name_        sql.NullString      `db:"name"`
	Address_     sql.NullString      `db:"address"`
	Country_     sql.NullString      `db:"country"`
	Config_      null.Map[any]       `db:"config"`
	Role_        string              `db:"role"`
	LogPolicy    LogPolicy           `db:"log_policy"`

	OrgConfig_ null.Map[any] `db:"org_config"`
	OrgIsAnon_ bool          `db:"org_is_anon"`

	expiration time.Time
}

func (c *Channel) ID() courier.ChannelID            { return c.ID_ }
func (c *Channel) UUID() courier.ChannelUUID        { return c.UUID_ }
func (c *Channel) OrgID() OrgID                     { return c.OrgID_ }
func (c *Channel) OrgIsAnon() bool                  { return c.OrgIsAnon_ }
func (c *Channel) ChannelType() courier.ChannelType { return c.ChannelType_ }
func (c *Channel) Name() string                     { return c.Name_.String }
func (c *Channel) Schemes() []string                { return []string(c.Schemes_) }
func (c *Channel) Address() string                  { return c.Address_.String }

// ChannelAddress returns the address of this channel
func (c *Channel) ChannelAddress() courier.ChannelAddress {
	if !c.Address_.Valid {
		return courier.NilChannelAddress
	}

	return courier.ChannelAddress(c.Address_.String)
}

// Country returns the country code for this channel if any
func (c *Channel) Country() string { return c.Country_.String }

// IsScheme returns whether this channel serves only the passed in scheme
func (c *Channel) IsScheme(scheme string) bool {
	return len(c.Schemes_) == 1 && c.Schemes_[0] == scheme
}

// Roles returns the roles of this channel
func (c *Channel) Roles() []courier.ChannelRole {
	roles := []courier.ChannelRole{}
	for _, char := range strings.Split(c.Role_, "") {
		roles = append(roles, courier.ChannelRole(char))
	}
	return roles
}

// HasRole returns whether the passed in channel supports the passed role
func (c *Channel) HasRole(role courier.ChannelRole) bool {
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
	return c.StringConfigForKey(courier.ConfigCallbackDomain, fallbackDomain)
}

// getChannel will look up the channel with the passed in UUID and channel type.
// It will return an error if the channel does not exist or is not active.
func getChannel(ctx context.Context, db *sqlx.DB, channelType courier.ChannelType, channelUUID courier.ChannelUUID) (*Channel, error) {
	// look for the channel locally
	cachedChannel, localErr := getCachedChannel(channelType, channelUUID)

	// found it? return it
	if localErr == nil {
		return cachedChannel, nil
	}

	// look in our database instead
	channel, dbErr := loadChannelFromDB(ctx, db, channelType, channelUUID)

	// if it wasn't found in the DB, clear our cache and return that it wasn't found
	if dbErr == courier.ErrChannelNotFound {
		clearLocalChannel(channelUUID)
		return cachedChannel, fmt.Errorf("unable to find channel with type: %s and uuid: %s", channelType, channelUUID)
	}

	// if we had some other db error, return it if our cached channel was only just expired
	if dbErr != nil && localErr == courier.ErrChannelExpired {
		return cachedChannel, nil
	}

	// no cached channel, oh well, we fail
	if dbErr != nil {
		return nil, dbErr
	}

	// we found it in the db, cache it locally
	cacheChannel(channel)
	return channel, nil
}

const sqlLookupChannelFromUUID = `
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
	c.log_policy,
	o.config AS org_config,
	o.is_anon AS org_is_anon
  FROM channels_channel c
  JOIN orgs_org o ON c.org_id = o.id
 WHERE c.uuid = $1 AND c.is_active = TRUE AND c.org_id IS NOT NULL`

// ChannelForUUID attempts to look up the channel with the passed in UUID, returning it
func loadChannelFromDB(ctx context.Context, db *sqlx.DB, channelType courier.ChannelType, uuid courier.ChannelUUID) (*Channel, error) {
	channel := &Channel{UUID_: uuid}

	// select just the fields we need
	err := db.GetContext(ctx, channel, sqlLookupChannelFromUUID, uuid)

	// we didn't find a match
	if err == sql.ErrNoRows {
		return nil, courier.ErrChannelNotFound
	}

	// other error
	if err != nil {
		return nil, err
	}

	// is it the right type?
	if channelType != courier.AnyChannelType && channelType != channel.ChannelType() {
		return nil, courier.ErrChannelWrongType
	}

	// found it, return it
	return channel, nil
}

// getCachedChannel returns a Channel object for the passed in type and UUID.
func getCachedChannel(channelType courier.ChannelType, uuid courier.ChannelUUID) (*Channel, error) {
	// first see if the channel exists in our local cache
	cacheMutex.RLock()
	channel, found := channelCache[uuid]
	cacheMutex.RUnlock()

	if found {
		// if it was found but the type is wrong, that's an error
		if channelType != courier.AnyChannelType && channel.ChannelType() != channelType {
			return nil, courier.ErrChannelWrongType
		}

		// if we've expired, we return it with an error
		if channel.expiration.Before(time.Now()) {
			return channel, courier.ErrChannelExpired
		}

		return channel, nil
	}

	return nil, courier.ErrChannelNotFound
}

func cacheChannel(channel *Channel) {
	channel.expiration = time.Now().Add(localTTL)

	cacheMutex.Lock()
	channelCache[channel.UUID()] = channel
	cacheMutex.Unlock()
}

func clearLocalChannel(uuid courier.ChannelUUID) {
	cacheMutex.Lock()
	delete(channelCache, uuid)
	cacheMutex.Unlock()
}

// channels stay cached in memory for a minute at a time
const localTTL = 60 * time.Second

var cacheMutex sync.RWMutex
var channelCache = make(map[courier.ChannelUUID]*Channel)

// getChannelByAddress will look up the channel with the passed in address and channel type.
// It will return an error if the channel does not exist or is not active.
func getChannelByAddress(ctx context.Context, db *sqlx.DB, channelType courier.ChannelType, address courier.ChannelAddress) (*Channel, error) {
	// look for the channel locally
	cachedChannel, localErr := getCachedChannelByAddress(channelType, address)

	// found it? return it
	if localErr == nil {
		return cachedChannel, nil
	}

	// look in our database instead
	channel, dbErr := loadChannelByAddressFromDB(ctx, db, channelType, address)

	// if it wasn't found in the DB, clear our cache and return that it wasn't found
	if dbErr == courier.ErrChannelNotFound {
		clearLocalChannelByAddress(address)
		return cachedChannel, fmt.Errorf("unable to find channel with type: %s and address: %s", string(channelType), address.String())
	}

	// if we had some other db error, return it if our cached channel was only just expired
	if dbErr != nil && localErr == courier.ErrChannelExpired {
		return cachedChannel, nil
	}

	// no cached channel, oh well, we fail
	if dbErr != nil {
		return nil, dbErr
	}

	// we found it in the db, cache it locally
	cacheChannelByAddress(channel)
	return channel, nil
}

const sqlLookupChannelFromAddress = `
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
	c.log_policy,
	o.config AS org_config,
	o.is_anon AS org_is_anon
  FROM channels_channel c
  JOIN orgs_org o ON c.org_id = o.id
 WHERE c.address = $1 AND c.is_active = TRUE AND c.org_id IS NOT NULL`

// loadChannelByAddressFromDB get the channel with the passed in channel type and address from the DB, returning it
func loadChannelByAddressFromDB(ctx context.Context, db *sqlx.DB, channelType courier.ChannelType, address courier.ChannelAddress) (*Channel, error) {
	channel := &Channel{Address_: sql.NullString{String: address.String(), Valid: address == courier.NilChannelAddress}}

	// select just the fields we need
	err := db.GetContext(ctx, channel, sqlLookupChannelFromAddress, address)

	// we didn't find a match
	if err == sql.ErrNoRows {
		return nil, courier.ErrChannelNotFound
	}

	// other error
	if err != nil {
		return nil, err
	}

	// is it the right type?
	if channelType != courier.AnyChannelType && channelType != channel.ChannelType() {
		return nil, courier.ErrChannelWrongType
	}

	// found it, return it
	return channel, nil
}

// getCachedChannelByAddress returns a Channel object for the passed in type and address.
func getCachedChannelByAddress(channelType courier.ChannelType, address courier.ChannelAddress) (*Channel, error) {
	// first see if the channel exists in our local cache
	cacheByAddressMutex.RLock()
	channel, found := channelByAddressCache[address]
	cacheByAddressMutex.RUnlock()

	// do not consider the cache for empty addresses
	if found && address != courier.NilChannelAddress {
		// if it was found but the type is wrong, that's an error
		if channelType != courier.AnyChannelType && channel.ChannelType() != channelType {
			return nil, courier.ErrChannelWrongType
		}

		// if we've expired, we return it with an error
		if channel.expiration.Before(time.Now()) {
			return channel, courier.ErrChannelExpired
		}

		return channel, nil
	}

	return nil, courier.ErrChannelNotFound
}

func cacheChannelByAddress(channel *Channel) {
	channel.expiration = time.Now().Add(localTTL)

	// never cache if the address is nil or empty
	if channel.ChannelAddress() != courier.NilChannelAddress {
		return
	}

	cacheByAddressMutex.Lock()
	channelByAddressCache[channel.ChannelAddress()] = channel
	cacheByAddressMutex.Unlock()
}

func clearLocalChannelByAddress(address courier.ChannelAddress) {
	cacheByAddressMutex.Lock()
	delete(channelByAddressCache, address)
	cacheByAddressMutex.Unlock()
}

var cacheByAddressMutex sync.RWMutex
var channelByAddressCache = make(map[courier.ChannelAddress]*Channel)
