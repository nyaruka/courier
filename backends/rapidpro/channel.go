package rapidpro

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
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
func (c *Channel) Country() i18n.Country { return i18n.Country(c.Country_.String) }

// IsScheme returns whether this channel serves only the passed in scheme
func (c *Channel) IsScheme(scheme *urns.Scheme) bool {
	return len(c.Schemes_) == 1 && c.Schemes_[0] == scheme.Prefix
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

func (b *backend) loadChannelByUUID(ctx context.Context, uuid courier.ChannelUUID) (*Channel, error) {
	channel := &Channel{}
	err := b.db.GetContext(ctx, channel, sqlLookupChannelFromUUID, uuid)

	if err == sql.ErrNoRows {
		return nil, courier.ErrChannelNotFound
	}
	return channel, err
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

func (b *backend) loadChannelByAddress(ctx context.Context, address courier.ChannelAddress) (*Channel, error) {
	channel := &Channel{}
	err := b.db.GetContext(ctx, channel, sqlLookupChannelFromAddress, address)

	if err == sql.ErrNoRows {
		return nil, courier.ErrChannelNotFound
	}
	return channel, err
}
