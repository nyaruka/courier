package novo

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

var testChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NV", "2020", "TT",
	[]string{urns.Phone.Prefix},
	map[string]any{
		"merchant_id":     "my-merchant-id",
		"merchant_secret": "my-merchant-secret",
		"secret":          "sesame",
	})

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{testChannel}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160

	RunOutgoingTests(t, testChannel, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"my-merchant-secret", "sesame"}})
}
