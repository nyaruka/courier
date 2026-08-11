package test

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
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
		Config_:      null.Map[any](asDecodedJSON(config)),
		Role_:        string(models.ChannelRoleSend) + string(models.ChannelRoleReceive),
		OrgConfig_:   null.Map[any]{},
	}
}

// SetChannelConfig sets a config value on a test channel, for values which aren't known until the test is running
// (e.g. the URL of a test server).
func SetChannelConfig(ch *models.Channel, key string, value any) {
	ch.Config_[key] = asDecodedJSON(value)
}

// Real channel config is a JSON column decoded into null.Map[any], so whole numbers reach handlers as float64 rather
// than int, and the accessors on Channel are written for that. Test authors naturally write Go literals, so round
// their config through JSON to give it the same shape at any nesting depth, rather than making those accessors
// tolerate types they never actually see.
func asDecodedJSON[T any](v T) T {
	var decoded T
	jsonx.MustUnmarshal(jsonx.MustMarshal(v), &decoded)
	return decoded
}
