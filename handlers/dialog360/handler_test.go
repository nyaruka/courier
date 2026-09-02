package dialog360

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

var testChannels = []*models.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"D3C",
		"250788383383",
		"RW",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			"auth_token": "the-auth-token",
			"base_url":   "https://waba-v2.360dialog.io",
		}),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newWAHandler(models.ChannelType("D3C"), "360Dialog"), "testdata/incoming.json", nil)
}

func TestBuildAttachmentRequest(t *testing.T) {
	d3CHandler := newWAHandler(models.ChannelType("D3C"), "360Dialog")(nil, channels.NewRoutes()).(*handler)
	req, _ := d3CHandler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "the-auth-token", req.Header.Get("D360-API-KEY"))

}

func TestOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3C", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{
		"auth_token": "the-auth-token",
		"base_url":   "https://waba-v2.360dialog.io",
	})

	RunOutgoingTests(t, ch, newWAHandler(models.ChannelType("D3C"), "360Dialog"), "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"the-auth-token"}})
}

func TestSendEvent(t *testing.T) {
	channel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3C", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{
		"auth_token": "the-auth-token",
		"base_url":   "https://waba-v2.360dialog.io",
	})

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	h := s.MountHandler(newWAHandler(models.ChannelType("D3C"), "360Dialog")).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://waba-v2.360dialog.io/messages": {
			httpx.NewMockResponse(200, nil, []byte(`{"success": true}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"error": {"message": "(#131009) Parameter value is not valid", "code": 131009}}`)),
			httpx.MockConnectionError,
		},
	})

	// typing indicators are supported but there's no explicit stop
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 20 * time.Second}, h.SendableEvents(channel))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "360Dialog")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:250788123123", "wamid.HBgMNTU3")

	// a typing indicator is sent as a mark-as-read call with a typing_indicator field
	clog := models.NewChannelLogForEventSend(channel, nil)
	err := h.SendEvent(context.Background(), channel, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://waba-v2.360dialog.io/messages", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, `{"messaging_product":"whatsapp","status":"read","message_id":"wamid.HBgMNTU3","typing_indicator":{"type":"text"}}`)

	// an error response is a response error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), channel, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// an event without a msg external ID can't be sent
	err = h.SendEvent(context.Background(), channel, events.NewTypingStarted(events.DirectionOutgoing, channelRef, "whatsapp:250788123123", ""), clog)
	assert.ErrorContains(t, err, "requires msg_external_id")

	// a channel without an auth token config can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3C", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{"base_url": "https://waba-v2.360dialog.io"})
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// nor can an event type the handler doesn't declare support for
	err = h.SendEvent(context.Background(), channel, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "whatsapp:250788123123", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}
