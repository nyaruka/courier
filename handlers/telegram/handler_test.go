package telegram

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

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TG", "2020", "US", []string{urns.Telegram.Prefix}, map[string]any{"auth_token": "a123"}),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		[]string{urns.Telegram.Prefix},
		map[string]any{models.ConfigAuthToken: "auth_token"},
	)

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"auth_token"}})
}

func TestSendEvent(t *testing.T) {
	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	h := s.MountHandler(newHandler).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.telegram.org/botauth_token/sendChatAction": {
			httpx.NewMockResponse(200, nil, []byte(`{"ok": true, "result": true}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"ok": false, "error_code": 400, "description": "Bad Request"}`)),
			httpx.MockConnectionError,
		},
	})

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		[]string{urns.Telegram.Prefix},
		map[string]any{models.ConfigAuthToken: "auth_token"},
	)

	typing := events.NewTypingStarted(events.DirectionOutgoing, assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "Telegram"), "telegram:12345", "")

	clog := models.NewChannelLogForEventSend(ch, nil)
	err := h.SendEvent(context.Background(), ch, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://api.telegram.org/botauth_token/sendChatAction", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, "chat_id=12345")
	assert.Contains(t, clog.HttpLogs[0].Request, "action=typing")

	// typing indicators display for ~5 seconds so should be resent more often than that to sustain
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 4 * time.Second}, h.SendableEvents(ch))

	// non-ok response is a response error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// channel without an auth token can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US", []string{urns.Telegram.Prefix}, map[string]any{})
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// an event type the handler doesn't declare support for can't be sent
	err = h.SendEvent(context.Background(), ch, events.NewTypingStopped(events.DirectionOutgoing, nil, "telegram:12345", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}

func TestIsValidButtonURL(t *testing.T) {
	valid := []string{
		"http://example.com",
		"https://example.com/path?x=1#y",
		"https://example.com:8080/path",
		"http://127.0.0.1/dev",
		"http://[::1]/dev",
		"https://exämple.com/ü",
		"tg://user?id=123456",
		"tg://resolve?domain=example",
	}
	for _, s := range valid {
		assert.True(t, isValidButtonURL(s), "expected %s to be valid", s)
	}

	invalid := []string{
		"",
		"example.com",
		"www.example.com/path",
		"mailto:bob@example.com",
		"ftp://example.com/file",
		"http://",
		"http://localhost/dev",
		"https://example.com/a page",
		"https://example .com",
	}
	for _, s := range invalid {
		assert.False(t, isValidButtonURL(s), "expected %s to be invalid", s)
	}
}
