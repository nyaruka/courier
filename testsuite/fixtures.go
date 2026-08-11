package testsuite

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/stretchr/testify/require"
)

// InsertChannel inserts (or updates by UUID) the given channel in the test database, setting its id on success.
// Test channels are built in memory (see test.NewMockChannel) so this is what gives them a database row that the
// read and write paths can resolve.
func InsertChannel(t testing.TB, rt *runtime.Runtime, ch *models.Channel) {
	// test channels are created with explicit ids in the seed data without advancing the sequence, so keep it ahead
	rt.DB.MustExec(`SELECT setval('channels_channel_id_seq', GREATEST((SELECT MAX(id) FROM channels_channel), 100))`)

	err := rt.DB.Get(&ch.ID_,
		`INSERT INTO channels_channel(uuid, org_id, channel_type, name, schemes, address, country, config, role, is_active, created_on, modified_on)
		      VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, NOW(), NOW())
		 ON CONFLICT (uuid) DO UPDATE SET org_id = EXCLUDED.org_id, channel_type = EXCLUDED.channel_type, name = EXCLUDED.name,
			schemes = EXCLUDED.schemes, address = EXCLUDED.address, country = EXCLUDED.country, config = EXCLUDED.config,
			role = EXCLUDED.role, is_active = TRUE
		 RETURNING id`,
		ch.UUID_, ch.OrgID_, ch.ChannelType_, ch.Name_.String, ch.Schemes_, ch.Address_, ch.Country_, jsonx.MustMarshal(ch.Config_), ch.Role_,
	)
	require.NoError(t, err)

	models.FlushChannelCache()
}

// InsertMedia inserts the given media (and its alternates) into the test database so it can be resolved by
// models.ResolveMedia.
func InsertMedia(t testing.TB, rt *runtime.Runtime, media *models.Media) {
	originalID := insertMedia(t, rt, media, nil)

	for _, alt := range media.Alternates_ {
		insertMedia(t, rt, alt, &originalID)
	}
}

// inserts (or updates by UUID, since the seed data has media of its own) a single media record
func insertMedia(t testing.TB, rt *runtime.Runtime, media *models.Media, originalID *int64) int64 {
	var id int64
	err := rt.DB.Get(&id,
		`INSERT INTO msgs_media(uuid, org_id, content_type, url, path, size, duration, width, height, original_id)
		      VALUES($1, 1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (uuid) DO UPDATE SET content_type = EXCLUDED.content_type, url = EXCLUDED.url, path = EXCLUDED.path,
			size = EXCLUDED.size, duration = EXCLUDED.duration, width = EXCLUDED.width, height = EXCLUDED.height,
			original_id = EXCLUDED.original_id
		 RETURNING id`,
		media.UUID_, media.ContentType_, media.URL_, media.Path_, media.Size_, media.Duration_, media.Width_, media.Height_, originalID,
	)
	require.NoError(t, err)
	return id
}
