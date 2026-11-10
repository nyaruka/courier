package highconnection

import (
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
		})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"Password"}})
}

func TestUCS2Segments(t *testing.T) {
	assert.Equal(t, 1, ucs2Segments(""))
	assert.Equal(t, 1, ucs2Segments(strings.Repeat("a", 70)))
	assert.Equal(t, 2, ucs2Segments(strings.Repeat("a", 71)))
	assert.Equal(t, 1, ucs2Segments(strings.Repeat("😀", 35))) // 70 code units
	assert.Equal(t, 2, ucs2Segments(strings.Repeat("😀", 36))) // 72 code units
}
