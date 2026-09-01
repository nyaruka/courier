package justcall

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func newChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JCL", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{models.ConfigAPIKey: "api_key", models.ConfigSecret: "api_secret"})
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newChannel()}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, newChannel(), newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"api_key", "api_secret"}})
}
