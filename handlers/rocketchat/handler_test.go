package rocketchat

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

var testChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "RC", "1234", "",
	[]string{urns.RocketChat.Prefix},
	map[string]any{
		configBaseURL:     "https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c",
		configSecret:      "123456789",
		configBotUsername: "rocket.cat",
	},
)

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{testChannel}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, testChannel, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"123456789"}})
}
