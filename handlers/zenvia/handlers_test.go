package zenvia

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func newWhatsAppChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVW", "2020", "BR", []string{urns.WhatsApp.Prefix}, map[string]any{"api_key": "zv-api-token"})
}

func newSMSChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVS", "2020", "BR", []string{urns.Phone.Prefix}, map[string]any{"api_key": "zv-api-token"})
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newWhatsAppChannel()}, newHandler("ZVW", "Zenvia WhatsApp"), "testdata/incoming_whatsapp.json", nil)
	RunIncomingTests(t, []*models.Channel{newSMSChannel()}, newHandler("ZVS", "Zenvia SMS"), "testdata/incoming_sms.json", nil)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	opts := &OutgoingOptions{CheckRedacted: []string{"zv-api-token"}}

	RunOutgoingTests(t, newWhatsAppChannel(), newHandler("ZVW", "Zenvia WhatsApp"), "testdata/outgoing_whatsapp.json", opts)
	RunOutgoingTests(t, newSMSChannel(), newHandler("ZVS", "Zenvia SMS"), "testdata/outgoing_sms.json", opts)
}
