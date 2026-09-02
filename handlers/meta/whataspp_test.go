package meta

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
)

var whatsappTestChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "1234567890", "", []string{urns.WhatsApp.Prefix}, map[string]any{models.ConfigAuthToken: "a123"}),
}

func TestWhatsAppIncoming(t *testing.T) {
	RunIncomingTests(t, whatsappTestChannels, newHandler("WAC", "Cloud API WhatsApp"), "testdata/whatsapp_incoming.json", &IncomingOptions{Sign: addValidSignature, NoInvalidChannelCheck: true})
}

func TestWhatsAppOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLengthWAC = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{models.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTests(t, ch, newHandler("WAC", "Cloud API WhatsApp"), "testdata/whatsapp_outgoing.json", &OutgoingOptions{CheckRedacted: checkRedacted})
}

func TestWhatsAppDescribeURN(t *testing.T) {
	channel := whatsappTestChannels[0]
	handler := newServerWithWAC().MountHandler(newHandler("WAC", "Cloud API WhatsApp"))
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, channel, nil, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"whatsapp:1337", map[string]string{}},
		{"whatsapp:4567", map[string]string{}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(models.URNDescriber).DescribeURN(context.Background(), whatsappTestChannels[0], tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestWhatsAppBuildAttachmentRequest(t *testing.T) {
	s := newServerWithWAC()
	handler := s.MountHandler(newHandler("WAC", "WhatsApp Cloud")).(*handler)
	req, _ := handler.BuildAttachmentRequest(context.Background(), whatsappTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer wac_admin_system_user_token", req.Header.Get("Authorization"))
}

func newServerWithWAC() *web.Server {
	cfg := runtime.NewDefaultConfig()
	cfg.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"
	return web.NewServer(runtime.NewTestRuntime(cfg))
}

func TestWhatsAppSendEvent(t *testing.T) {
	// other tests repoint graphURL at mock servers, so pin it for this test
	defer func(u string) { graphURL = u }(graphURL)
	graphURL = "https://graph.facebook.com/v25.0/"

	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{models.ConfigAuthToken: "a123"})

	cfg := runtime.NewDefaultConfig()
	cfg.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"
	s := web.NewServer(runtime.NewTestRuntime(cfg))

	h := s.MountHandler(newHandler("WAC", "WhatsApp Cloud")).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://graph.facebook.com/12345_ID/messages": {
			httpx.NewMockResponse(200, nil, []byte(`{"success": true}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"error": {"message": "(#131009) Parameter value is not valid", "code": 131009}}`)),
			httpx.MockConnectionError,
		},
	})

	// typing indicators are supported on WhatsApp channels, but there's no explicit stop
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 20 * time.Second}, h.SendableEvents(channel))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WhatsApp")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:5511987654321", "wamid.HBgMNTU3")

	// a typing indicator is sent as a mark-as-read call with a typing_indicator field
	clog := models.NewChannelLogForEventSend(channel, nil)
	err := h.SendEvent(context.Background(), channel, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://graph.facebook.com/12345_ID/messages", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, `{"messaging_product":"whatsapp","status":"read","message_id":"wamid.HBgMNTU3","typing_indicator":{"type":"text"}}`)

	// an error response is a response error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// an event without a msg external ID can't be sent
	err = h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:5511987654321", ""), clog)
	assert.ErrorContains(t, err, "requires msg_external_id")

	// nor can an event type the handler doesn't declare support for
	err = h.SendEvent(context.Background(), channel, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "whatsapp:5511987654321", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}
