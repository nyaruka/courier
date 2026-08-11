package test_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

// test channels should carry config in the same shape a database-loaded channel does, so that handlers reading it
// exercise the same accessor paths - notably whole numbers as float64 rather than int, at any nesting depth
func TestMockChannelConfigIsJSONShaped(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "1234", "US", []string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigMaxLength: 25,
			"nested":               map[string]any{"count": 7},
			"list":                 []any{1, "two"},
		})

	assert.Equal(t, float64(25), ch.ConfigForKey(models.ConfigMaxLength, nil))
	assert.Equal(t, 25, ch.IntConfigForKey(models.ConfigMaxLength, -1))
	assert.Equal(t, map[string]any{"count": float64(7)}, ch.ConfigForKey("nested", nil))
	assert.Equal(t, []any{float64(1), "two"}, ch.ConfigForKey("list", nil))

	test.SetChannelConfig(ch, "added", 12)
	assert.Equal(t, float64(12), ch.ConfigForKey("added", nil))

	// a nil config is usable rather than a nil map SetChannelConfig would panic on
	bare := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "1234", "US", []string{urns.Phone.Prefix}, nil)
	test.SetChannelConfig(bare, "url", "http://example.com")
	assert.Equal(t, "http://example.com", bare.StringConfigForKey("url", ""))
}
