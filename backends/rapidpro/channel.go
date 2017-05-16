package rapidpro

import (
	"database/sql"
	"sync"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
)

// ChannelID is our SQL type for a channel's id
type ChannelID struct {
	sql.NullInt64
}

// NilChannelID is our nil value for ChannelIDs
var NilChannelID = ChannelID{sql.NullInt64{Int64: 0, Valid: false}}

// getChannelFromUUID will look up the channel with the passed in UUID and channel type.
// It will return an error if the channel does not exist or is not active.
func getChannel(b *backend, channelType courier.ChannelType, channelUUID courier.ChannelUUID) (courier.Channel, error) {
	// look for the channel locally
	channel, localErr := getLocalChannel(channelType, channelUUID)

	// found it? return it
	if localErr == nil {
		return channel, nil
	}

	// look in our database instead
	dbErr := loadChannelFromDB(b, channel, channelType, channelUUID)

	// if it wasn't found in the DB, clear our cache and return that it wasn't found
	if dbErr == courier.ErrChannelNotFound {
		clearLocalChannel(channelUUID)
		return channel, dbErr
	}

	// if we had some other db error, return it if our cached channel was only just expired
	if dbErr != nil && localErr == courier.ErrChannelExpired {
		return channel, nil
	}

	// no cached channel, oh well, we fail
	if dbErr != nil {
		return nil, dbErr
	}

	// we found it in the db, cache it locally
	cacheLocalChannel(channel)
	return channel, nil
}

const lookupChannelFromUUIDSQL = `
SELECT org_id, id, uuid, channel_type, address, country, config 
FROM channels_channel 
WHERE channel_type = $1 AND uuid = $2 AND is_active = true`

// ChannelForUUID attempts to look up the channel with the passed in UUID, returning it
func loadChannelFromDB(b *backend, channel *Channel, channelType courier.ChannelType, uuid courier.ChannelUUID) error {
	// select just the fields we need
	err := b.db.Get(channel, lookupChannelFromUUIDSQL, channelType, uuid)

	// we didn't find a match
	if err == sql.ErrNoRows {
		return courier.ErrChannelNotFound
	}

	// other error
	if err != nil {
		return err
	}

	// found it, return it
	return nil
}

// getLocalChannel returns a Channel object for the passed in type and UUID.
func getLocalChannel(channelType courier.ChannelType, uuid courier.ChannelUUID) (*Channel, error) {
	// first see if the channel exists in our local cache
	cacheMutex.RLock()
	channel, found := channelCache[uuid]
	cacheMutex.RUnlock()

	if found {
		// if it was found but the type is wrong, that's an error
		if channel.ChannelType() != channelType {
			return &Channel{ChannelType_: channelType, UUID_: uuid}, courier.ErrChannelWrongType
		}

		// if we've expired, clear our cache and return it
		if channel.expiration.Before(time.Now()) {
			return channel, courier.ErrChannelExpired
		}

		return channel, nil
	}

	return &Channel{ChannelType_: channelType, UUID_: uuid}, courier.ErrChannelNotFound
}

func cacheLocalChannel(channel *Channel) {
	// set our expiration
	channel.expiration = time.Now().Add(localTTL * time.Second)

	// first write to our local cache
	cacheMutex.Lock()
	channelCache[channel.UUID()] = channel
	cacheMutex.Unlock()
}

func clearLocalChannel(uuid courier.ChannelUUID) {
	cacheMutex.Lock()
	delete(channelCache, uuid)
	cacheMutex.Unlock()
}

const localTTL = 60

var cacheMutex sync.RWMutex
var channelCache = make(map[courier.ChannelUUID]*Channel)

//-----------------------------------------------------------------------------
// Channel Implementation
//-----------------------------------------------------------------------------

// Channel is the RapidPro specific concrete type satisfying the courier.Channel interface
type Channel struct {
	OrgID_       OrgID               `db:"org_id"`
	ID_          ChannelID           `db:"id"`
	ChannelType_ courier.ChannelType `db:"channel_type"`
	UUID_        courier.ChannelUUID `db:"uuid"`
	Address_     sql.NullString      `db:"address"`
	Country_     sql.NullString      `db:"country"`
	Config_      utils.NullMap       `db:"config"`

	expiration time.Time
}

// OrgID returns the id of the org this channel is for
func (c *Channel) OrgID() OrgID { return c.OrgID_ }

// ChannelType returns the type of this channel
func (c *Channel) ChannelType() courier.ChannelType { return c.ChannelType_ }

// ID returns the id of this channel
func (c *Channel) ID() ChannelID { return c.ID_ }

// UUID returns the UUID of this channel
func (c *Channel) UUID() courier.ChannelUUID { return c.UUID_ }

// Address returns the address of this channel
func (c *Channel) Address() string { return c.Address_.String }

// Country returns the country code for this channel if any
func (c *Channel) Country() string { return c.Country_.String }

// ConfigForKey returns the config value for the passed in key, or defaultValue if it isn't found
func (c *Channel) ConfigForKey(key string, defaultValue interface{}) interface{} {
	// no value, return our default value
	if !c.Config_.Valid {
		return defaultValue
	}

	value, found := c.Config_.Map[key]
	if !found {
		return defaultValue
	}
	return value
}
