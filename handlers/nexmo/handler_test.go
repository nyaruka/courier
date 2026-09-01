package nexmo

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configNexmoAPIKey:        "nexmo-api-key",
			configNexmoAPISecret:     "nexmo-api-secret",
			configNexmoAppID:         "nexmo-app-id",
			configNexmoAppPrivateKey: "nexmo-app-private-key",
		})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"nexmo-api-secret", "nexmo-app-private-key"}})
}
