package test

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/null/v3"
)

// NewMockChannel creates a channel for tests which isn't backed by the database. It's a real *models.Channel so that
// handlers exercise the same config and scheme logic they do in production - it just isn't loaded from, or writable
// to, channels_channel, and so has no id.
func NewMockChannel(uuid string, channelType string, address string, country i18n.Country, schemes []string, config map[string]any) *models.Channel {
	if config == nil {
		config = map[string]any{}
	}

	return &models.Channel{
		OrgID_:       1,
		UUID_:        models.ChannelUUID(uuid),
		ChannelType_: models.ChannelType(channelType),
		Schemes_:     pq.StringArray(schemes),
		Name_:        sql.NullString{String: fmt.Sprintf("Channel: %s", uuid), Valid: true},
		Address_:     sql.NullString{String: address, Valid: address != ""},
		Country_:     sql.NullString{String: string(country), Valid: country != ""},
		Config_:      null.Map[any](normalizeConfig(config)),
		Role_:        string(models.ChannelRoleSend) + string(models.ChannelRoleReceive),
		OrgConfig_:   null.Map[any]{},
	}
}

// SetChannelConfig sets a config value on a test channel, for values which aren't known until the test is running
// (e.g. the URL of a test server).
func SetChannelConfig(ch *models.Channel, key string, value any) {
	ch.Config_[key] = normalizeValue(value)
}

// Real channel config is JSON decoded out of Postgres, so whole numbers arrive as float64 and are read back with
// IntConfigForKey. Test authors naturally write literal ints, so convert them to match production's shape rather
// than making the production accessor tolerate a type it never actually sees.
func normalizeConfig(config map[string]any) map[string]any {
	normalized := make(map[string]any, len(config))
	for k, v := range config {
		normalized[k] = normalizeValue(v)
	}
	return normalized
}

func normalizeValue(v any) any {
	if i, ok := v.(int); ok {
		return float64(i)
	}
	return v
}
