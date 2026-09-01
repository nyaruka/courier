package playmobile

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
			[]string{urns.Phone.Prefix},
			map[string]any{
				"incoming_prefixes": []string{"abc", "DE"},
			},
		),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":  "Password",
			"username":  "Username",
			"shortcode": "1122",
			"base_url":  "http://example.com",
		})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{httpx.BasicAuth("Username", "Password")}})
}
