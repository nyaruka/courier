package kaleyra

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func newChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "KWA", "250788383383", "",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{configAccountSID: "SID", configApiKey: "123456"},
	)
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newChannel()}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, newChannel(), newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"123456"}})
}
