package whatsapp_legacy

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

// each channel type needs its own channel because they're all written to the test database
var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WA", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568d", "D3", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568e", "TXW", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
}

func buildTestCases(receiveURL string) []IncomingTestCase {
	return []IncomingTestCase{
		{
			Label:                 "Receive Message Ignored",
			URL:                   receiveURL,
			Data:                  `{"messages": [{"from": "250788123123", "id": "41", "timestamp": "1454119029", "text": {"body": "hello world"}, "type": "text"}]}`,
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "Events Handled",
			NoInvalidChannelCheck: true,
		},
		{
			Label:                 "Receive Status Ignored",
			URL:                   receiveURL,
			Data:                  `{"statuses": [{"id": "9712A34B4A8B6AD50F", "recipient_id": "16315555555", "status": "sent", "timestamp": "1518694700"}]}`,
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "Events Handled",
			NoInvalidChannelCheck: true,
		},
	}
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("WA"), "WhatsApp"), buildTestCases("/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"))
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("D3"), "360Dialog"), buildTestCases("/c/d3/8eb23e93-5ecb-45ba-b726-3b064e0c568d/receive"))
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("TXW"), "TextIt"), buildTestCases("/c/txw/8eb23e93-5ecb-45ba-b726-3b064e0c568e/receive"))
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newWAHandler(models.ChannelType("WA"), "WhatsApp"), []OutgoingTestCase{
		{
			Label:         "Disabled Send",
			MsgText:       "hello",
			MsgURN:        "whatsapp:250788123123",
			ExpectedError: channels.ErrFailedWithReason("disabled", "WhatsApp legacy handler is disabled"),
		},
	}, nil, nil)
}
