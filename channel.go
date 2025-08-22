package courier

import (
	"errors"

	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
)

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
	UUID() models.ChannelUUID
	Name() string
	ChannelType() models.ChannelType
	Schemes() []string
	Country() i18n.Country
	Address() string
	ChannelAddress() models.ChannelAddress

	Roles() []models.ChannelRole

	// is this channel for the passed in scheme (and only that scheme)
	IsScheme(*urns.Scheme) bool

	// CallbackDomain returns the domain that should be used for any callbacks the channel registers
	CallbackDomain(fallbackDomain string) string

	ConfigForKey(key string, defaultValue any) any
	StringConfigForKey(key string, defaultValue string) string
	BoolConfigForKey(key string, defaultValue bool) bool
	IntConfigForKey(key string, defaultValue int) int
	OrgConfigForKey(key string, defaultValue any) any
}
