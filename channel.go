package courier

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nyaruka/courier/utils"
	uuid "github.com/satori/go.uuid"
)

const (
	// ConfigAuthToken is our constant key used in channel configs for auth tokens
	ConfigAuthToken = "auth_token"
)

// ChannelID is our SQL type for a channel's id
type ChannelID struct {
	sql.NullInt64
}

// NilChannelID is our nil value for ChannelIDs
var NilChannelID = ChannelID{sql.NullInt64{Int64: 0, Valid: false}}

// ChannelType is our typing of the two char channel types
type ChannelType string

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
func ChannelFromUUID(s *server, channelType ChannelType, uuidStr string) (*Channel, error) {
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

const lookupChannelFromUUIDSQL = `
SELECT org_id, id, uuid, channel_type, address, country, config 
FROM channels_channel 
WHERE channel_type = $1 AND uuid = $2 AND is_active = true`

// ChannelForUUID attempts to look up the channel with the passed in UUID, returning it
func loadChannelFromDB(s *server, channel *Channel, channelType ChannelType, uuid ChannelUUID) error {
	// select just the fields we need
	err := s.db.Get(channel, lookupChannelFromUUIDSQL, channelType, uuid)

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
var channelCache = make(map[ChannelUUID]*Channel)

// getLocalChannel returns a Channel object for the passed in type and UUID.
func getLocalChannel(channelType ChannelType, uuid ChannelUUID) (*Channel, error) {
	// first see if the channel exists in our local cache
	cacheMutex.RLock()
	channel, found := channelCache[uuid]
	cacheMutex.RUnlock()

	if found {
		// if it was found but the type is wrong, that's an error
		if channel.ChannelType != channelType {
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

func cacheLocalChannel(channel *Channel) {
	// set our expiration
	channel.expiration = time.Now().Add(localTTL * time.Second)

	// first write to our local cache
	cacheMutex.Lock()
	channelCache[channel.UUID] = channel
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

// Channel is our struct for json and db representations of our channel
type Channel struct {
	OrgID       OrgID          `json:"org_id"        db:"org_id"`
	ID          ChannelID      `json:"id"            db:"id"`
	UUID        ChannelUUID    `json:"uuid"          db:"uuid"`
	ChannelType ChannelType    `json:"channel_type"  db:"channel_type"`
	Address     sql.NullString `json:"address"       db:"address"`
	Country     sql.NullString `json:"country"       db:"country"`
	Config      utils.NullDict `json:"config"        db:"config"`
	expiration  time.Time
}

// GetConfig returns the value of the passed in config key
func (c *Channel) GetConfig(key string) string {
	if c.Config.Valid {
		return c.Config.Dict[key]
	}
	return ""
}

// Constructor to create a new empty channel
func newChannel(channelType ChannelType, uuid ChannelUUID) *Channel {
	return &Channel{ChannelType: channelType, UUID: uuid}
}
