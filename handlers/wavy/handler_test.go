package wavy

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigUsername:  "user1",
			models.ConfigAuthToken: "token",
		},
	)

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"token"}})
}
