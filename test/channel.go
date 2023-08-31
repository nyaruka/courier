package test

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/urns"
)

// MockChannel implements the Channel interface and is used in our tests
type MockChannel struct {
	uuid        courier.ChannelUUID
	channelType courier.ChannelType
	schemes     []string
	address     courier.ChannelAddress
	country     string
	role        string
	config      map[string]any
	orgConfig   map[string]any
}

// UUID returns the uuid for this channel
func (c *MockChannel) UUID() courier.ChannelUUID { return c.uuid }

// Name returns the name of this channel, we just return our UUID for our mock instances
func (c *MockChannel) Name() string { return fmt.Sprintf("Channel: %s", c.uuid) }

// ChannelType returns the type of this channel
func (c *MockChannel) ChannelType() courier.ChannelType { return c.channelType }

// SetScheme sets the scheme for this channel
func (c *MockChannel) SetScheme(scheme string) { c.schemes = []string{scheme} }

// Schemes returns the schemes for this channel
func (c *MockChannel) Schemes() []string { return c.schemes }

// IsScheme returns whether the passed in scheme is the scheme for this channel
func (c *MockChannel) IsScheme(scheme string) bool {
	return len(c.schemes) == 1 && c.schemes[0] == scheme
}

// Address returns the address as a string of this channel
func (c *MockChannel) Address() string { return c.address.String() }

// ChannelAddress returns the address of this channel
func (c *MockChannel) ChannelAddress() courier.ChannelAddress { return c.address }

// Country returns the country this channel is for (if any)
func (c *MockChannel) Country() string { return c.country }

// SetConfig sets the passed in config parameter
func (c *MockChannel) SetConfig(key string, value any) {
	c.config[key] = value
}

// CallbackDomain returns the callback domain to use for this channel
func (c *MockChannel) CallbackDomain(fallbackDomain string) string {
	value, found := c.config[courier.ConfigCallbackDomain]
	if !found {
		return fallbackDomain
	}
	return value.(string)
}

// ConfigForKey returns the config value for the passed in key
func (c *MockChannel) ConfigForKey(key string, defaultValue any) any {
	value, found := c.config[key]
	if !found {
		return defaultValue
	}
	return value
}

// StringConfigForKey returns the config value for the passed in key
func (c *MockChannel) StringConfigForKey(key string, defaultValue string) string {
	val := c.ConfigForKey(key, defaultValue)
	str, isStr := val.(string)
	if !isStr {
		return defaultValue
	}
	return str
}

// BoolConfigForKey returns the config value for the passed in key
func (c *MockChannel) BoolConfigForKey(key string, defaultValue bool) bool {
	val := c.ConfigForKey(key, defaultValue)
	b, isBool := val.(bool)
	if !isBool {
		return defaultValue
	}
	return b
}

// IntConfigForKey returns the config value for the passed in key
func (c *MockChannel) IntConfigForKey(key string, defaultValue int) int {
	val := c.ConfigForKey(key, defaultValue)

	// golang unmarshals number literals in JSON into float64s by default
	f, isFloat := val.(float64)
	if isFloat {
		return int(f)
	}

	// test authors may use literal ints
	i, isInt := val.(int)
	if isInt {
		return i
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

// OrgConfigForKey returns the org config value for the passed in key
func (c *MockChannel) OrgConfigForKey(key string, defaultValue any) any {
	value, found := c.orgConfig[key]
	if !found {
		return defaultValue
	}
	return value
}

// SetRoles sets the role on the channel
func (c *MockChannel) SetRoles(roles []courier.ChannelRole) {
	c.role = fmt.Sprint(roles)
}

// Roles returns the roles of this channel
func (c *MockChannel) Roles() []courier.ChannelRole {
	roles := []courier.ChannelRole{}
	for _, char := range strings.Split(c.role, "") {
		roles = append(roles, courier.ChannelRole(char))
	}
	return roles
}

// HasRole returns whether the passed in channel supports the passed role
func (c *MockChannel) HasRole(role courier.ChannelRole) bool {
	for _, r := range c.Roles() {
		if r == role {
			return true
		}
	}
	return false
}

// NewMockChannel creates a new mock channel for the passed in type, address, country and config
func NewMockChannel(uuid string, channelType string, address string, country string, config map[string]any) *MockChannel {
	return &MockChannel{
		uuid:        courier.ChannelUUID(uuid),
		channelType: courier.ChannelType(channelType),
		schemes:     []string{urns.TelScheme},
		address:     courier.ChannelAddress(address),
		country:     country,
		config:      config,
		role:        "SR",
		orgConfig:   map[string]any{},
	}
}
