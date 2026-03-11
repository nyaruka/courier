package whatsapp_legacy

import (
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WA", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "D3", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TXW", "250788383383", "RW", []string{urns.WhatsApp.Prefix}, nil),
}

func buildTestCases(receiveURL string) []IncomingTestCase {
	return []IncomingTestCase{
		{
			Label:                 "Receive Message Ignored",
			URL:                   receiveURL,
			Data:                  `{"messages": [{"from": "250788123123", "id": "41", "timestamp": "1454119029", "text": {"body": "hello world"}, "type": "text"}]}`,
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "Events Handled",
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
		{
			Label:                 "Receive Status Ignored",
			URL:                   receiveURL,
			Data:                  `{"statuses": [{"id": "9712A34B4A8B6AD50F", "recipient_id": "16315555555", "status": "sent", "timestamp": "1518694700"}]}`,
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "Events Handled",
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
	}
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("WA"), "WhatsApp"), buildTestCases("/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"))
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("D3"), "360Dialog"), buildTestCases("/c/d3/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"))
	RunIncomingTestCases(t, testChannels, newWAHandler(models.ChannelType("TXW"), "TextIt"), buildTestCases("/c/txw/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"))
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newWAHandler(models.ChannelType("WA"), "WhatsApp"), []OutgoingTestCase{
		{
			Label:   "Noop Send",
			MsgText: "hello",
			MsgURN:  "whatsapp:250788123123",
		},
	}, nil, nil)
}
