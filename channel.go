package courier

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
)

const (
	ConfigAuthToken = "auth_token"
)

type ChannelType string

type ChannelUUID struct {
	uuid.UUID
}

var NilChannelUUID = ChannelUUID{uuid.Nil}

func NewChannelUUID(u string) (ChannelUUID, error) {
	channelUUID, err := uuid.FromString(strings.ToLower(u))
	if err != nil {
		return NilChannelUUID, err
	}
	return ChannelUUID{channelUUID}, nil
}

type Channel interface {
	UUID() ChannelUUID
	ChannelType() ChannelType
	Address() string
	Country() string
	GetConfig(string) string
}

// ChannelFromUUID will look up the channel with the passed in UUID and channel type.
// It will return an error if the channel does not exist or is not active.
//
// This will use a 3 tier caching strategy:
//  1) Process level cache, we will first check a local cache, which is expired
//     every 5 seconds
//  2) Redis level cache, we will consult Redis for the latest Channel definition, caching
//     it locally if found
//  3) Postgres Lookup, we will lookup the value in our database, caching the result
//     both locally and in Redis
func ChannelFromUUID(s *server, channelType ChannelType, uuidStr string) (Channel, error) {
	channelUUID, err := NewChannelUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	// look for the channel locally
	channel, localErr := getLocalChannel(channelType, channelUUID)

	// found it? return it
	if localErr == nil {
		return channel, nil
	}

	// look in our database instead
	dbErr := loadChannelFromDB(s, channel, channelType, channelUUID)

	// if it wasn't found in the DB, clear our cache and return that it wasn't found
	if dbErr == ErrChannelNotFound {
		clearLocalChannel(channelUUID)
		return channel, dbErr
	}

	// if we had some other db error, return it if our cached channel was only just expired
	if dbErr != nil && localErr == ErrChannelExpired {
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

const lookupChannelFromUUIDSQL = `SELECT uuid, channel_type, address, country, config 
FROM channels_channel 
WHERE channel_type = $1 AND uuid = $2 AND is_active = true`

// ChannelForUUID attempts to look up the channel with the passed in UUID, returning it
func loadChannelFromDB(s *server, channel *channel, channelType ChannelType, uuid ChannelUUID) error {
	// select just the fields we need
	err := s.db.Get(channel, lookupChannelFromUUIDSQL, channelType, uuid)

	// parse our config
	channel.parseConfig()

	// we didn't find a match
	if err == sql.ErrNoRows {
		return ErrChannelNotFound
	}

	// other error
	if err != nil {
		return err
	}

	// found it, return it
	return nil
}

var cacheMutex sync.RWMutex
var channelCache = make(map[ChannelUUID]*channel)

var ErrChannelExpired = errors.New("channel expired")
var ErrChannelNotFound = errors.New("channel not found")
var ErrChannelWrongType = errors.New("channel type wrong")

// getLocalChannel returns a Channel object for the passed in type and UUID.
func getLocalChannel(channelType ChannelType, uuid ChannelUUID) (*channel, error) {
	// first see if the channel exists in our local cache
	cacheMutex.RLock()
	channel, found := channelCache[uuid]
	cacheMutex.RUnlock()

	if found {
		// if it was found but the type is wrong, that's an error
		if channel.ChannelType() != channelType {
			return newChannel(channelType, uuid), ErrChannelWrongType
		}

		// if we've expired, clear our cache and return it
		if channel.expiration.Before(time.Now()) {
			return channel, ErrChannelExpired
		}

		return channel, nil
	}

	return newChannel(channelType, uuid), ErrChannelNotFound
}

func cacheLocalChannel(channel *channel) {
	// set our expiration
	channel.expiration = time.Now().Add(localTTL * time.Second)

	// first write to our local cache
	cacheMutex.Lock()
	channelCache[channel.UUID()] = channel
	cacheMutex.Unlock()
}

func clearLocalChannel(uuid ChannelUUID) {
	cacheMutex.Lock()
	delete(channelCache, uuid)
	cacheMutex.Unlock()
}

const redisTTL = 3600 * 24
const localTTL = 60

//-----------------------------------------------------------------------------
// Channel implementation
//-----------------------------------------------------------------------------

type channel struct {
	UUID_        ChannelUUID `db:"uuid"         json:"uuid"`
	ChannelType_ ChannelType `db:"channel_type" json:"channel_type"`
	Address_     string      `db:"address"      json:"address"`
	Country_     string      `db:"country"      json:"country"`
	Config_      string      `db:"config"       json:"config"`

	expiration time.Time
	config     map[string]string
}

func (c *channel) UUID() ChannelUUID           { return c.UUID_ }
func (c *channel) ChannelType() ChannelType    { return c.ChannelType_ }
func (c *channel) Address() string             { return c.Address_ }
func (c *channel) Country() string             { return c.Country_ }
func (c *channel) GetConfig(key string) string { return c.config[key] }

func (c *channel) parseConfig() {
	c.config = make(map[string]string)

	if c.Config_ != "" {
		err := json.Unmarshal([]byte(c.Config_), &c.config)
		if err != nil {
			log.Printf("ERROR parsing channel config '%s': %s", c.Config_, err)
		}
	}
}

func newChannel(channelType ChannelType, uuid ChannelUUID) *channel {
	config := make(map[string]string)
	return &channel{ChannelType_: channelType, UUID_: uuid, config: config}
}
