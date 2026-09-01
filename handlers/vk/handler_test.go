package vk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

const channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"

var testChannels = []*models.Channel{
	test.NewMockChannel(
		channelUUID,
		"VK",
		"123456789",
		"",
		[]string{urns.VK.Prefix},
		map[string]any{
			models.ConfigAuthToken:         "token123xyz",
			models.ConfigSecret:            "abc123xyz",
			configServerVerificationString: "a1b2c3",
		}),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

func buildMockVKService() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, actionGetUser) {
			userId := r.URL.Query()["user_ids"][0]

			if userId == "123456789" {
				_, _ = w.Write([]byte(`{"response": [{"id": 123456789, "first_name": "John", "last_name": "Doe"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"response": []}`))
		}
	}))

	apiBaseURL = server.URL

	return server
}

func TestDescribeURN(t *testing.T) {
	defer func(u string) { apiBaseURL = u }(apiBaseURL)
	server := buildMockVKService()
	defer server.Close()

	handler := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig())).MountHandler(newHandler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))
	urn, _ := urns.New(urns.VK, "123456789")
	data := map[string]string{"name": "John Doe"}

	describe, err := handler.(models.URNDescriber).DescribeURN(context.Background(), testChannels[0], urn, clog)
	assert.Nil(t, err)
	assert.Equal(t, data, describe)

	AssertChannelLogRedaction(t, clog, []string{"token123xyz", "abc123xyz"})
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, testChannels[0], newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"token123xyz", "abc123xyz"}})
}

func TestSendEvent(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VK", "2020", "US", []string{urns.VK.Prefix}, map[string]any{models.ConfigAuthToken: "token123xyz"})

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))
	h := s.MountHandler(newHandler).(*handler)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.vk.com/method/messages.setActivity.json*": {
			httpx.NewMockResponse(200, nil, []byte(`{"response": 1}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"error": {"error_code": 5, "error_msg": "User authorization failed"}}`)),
			httpx.NewMockResponse(400, nil, []byte(`bad request`)),
			httpx.MockConnectionError,
		},
	})

	// typing indicators are supported but there's no explicit stop
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 8 * time.Second}, h.SendableEvents(ch))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VK")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "vk:123456789", "")

	// a typing started event is sent as a typing activity
	clog := models.NewChannelLogForEventSend(ch, nil)
	err := h.SendEvent(context.Background(), ch, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Contains(t, clog.HttpLogs[0].URL, "https://api.vk.com/method/messages.setActivity.json")
	assert.Contains(t, clog.HttpLogs[0].URL, "type=typing")
	assert.Contains(t, clog.HttpLogs[0].URL, "user_id=123456789")

	// a VK error in a 200 response is a response error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a non-2XX response
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// and a connection error is a connection error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// a channel without an auth token config can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VK", "2020", "US", []string{urns.VK.Prefix}, nil)
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// nor can an event type the handler doesn't declare support for
	err = h.SendEvent(context.Background(), ch, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "vk:123456789", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}
