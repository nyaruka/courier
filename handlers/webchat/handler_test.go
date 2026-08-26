package webchat

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/random"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	channelUUID = "0665bf36-4d2e-4c3f-b8a1-9f8e6a5c2d71"
	startURL    = "/c/wch/" + channelUUID + "/start"
	receiveURL  = "/c/wch/" + channelUUID + "/receive"

	testChatID = "vM0GGhDrqpTQefIEinK0up3C" // what the seeded secure source below generates
)

var testChannels = []*models.Channel{
	test.NewMockChannel(channelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil),
}

var incomingCases = []IncomingTestCase{
	{
		Label:                "Start Chat",
		URL:                  startURL,
		Data:                 `{}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `{"chat_id":"` + testChatID + `"}`,
	},
	{
		Label:                "Receive Msg",
		URL:                  receiveURL,
		Data:                 `{"chat_id": "` + testChatID + `", "text": "Hello"}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Hello"),
		ExpectedURN:          urns.URN("webchat:" + testChatID),
	},
	{
		Label:                "Receive Unknown Chat ID",
		URL:                  receiveURL,
		Data:                 `{"chat_id": "Xx3dE5fG7hJ9kL1mN3pQ5rXX", "text": "Hello"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown chat id",
	},
	{
		Label:                "Receive Invalid Chat ID",
		URL:                  receiveURL,
		Data:                 `{"chat_id": "not-a-chat-id!", "text": "Hello"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid chat id",
	},
	{
		Label:                "Receive Missing Text",
		URL:                  receiveURL,
		Data:                 `{"chat_id": "` + testChatID + `"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'Text' failed on the 'required' tag",
	},
}

func TestIncoming(t *testing.T) {
	// seed chat ID generation so the started chat is the one the receive cases use
	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	RunIncomingTestCases(t, testChannels, newHandler(), incomingCases)
}

// the framework can't assert response headers or make OPTIONS requests, so CORS support is tested directly
func TestCORS(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	require.NoError(t, s.MountHandler(newHandler()))

	// preflights on both endpoints are answered without needing the channel
	for _, path := range []string{startURL, receiveURL} {
		req, _ := http.NewRequest(http.MethodOptions, "https://localhost"+path, nil)
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)

		assert.Equal(t, 204, rr.Code, path)
		assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"), path)
		assert.Equal(t, "POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"), path)
		assert.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"), path)
	}

	// actual responses carry the allow-origin header too, including error responses
	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+startURL, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	req, _ = http.NewRequest(http.MethodPost, "https://localhost"+receiveURL, strings.NewReader(`{}`))
	rr = httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

// sends don't make HTTP requests so the framework's outgoing cases don't fit - instead we test the socket
// publishes directly
func TestOutgoing(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	uuids.SetGenerator(uuids.NewSeededGenerator(1234, dates.NewSequentialNow(time.Date(2025, 10, 13, 11, 20, 0, 0, time.UTC), time.Second)))
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	dates.SetNowFunc(dates.NewFixedNow(time.Date(2025, 10, 13, 11, 20, 30, 0, time.UTC)))
	defer dates.SetNowFunc(time.Now)

	ch := testChannels[0]
	testsuite.InsertChannel(t, rt, ch)

	h := newHandler()
	h.SetRuntime(rt)

	msg := &models.MsgOut{
		OrgID_:       ch.OrgID(),
		UUID_:        "0191e180-7d60-7000-aded-7d8b151cbd5b",
		Contact_:     &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		URN_:         urns.URN("webchat:" + testChatID),
		Text_:        "Hello there",
		ChannelUUID_: ch.UUID(),
		Channel_:     ch,
	}

	socket := models.ChatSocket(ch.UUID(), testChatID)

	send := func() error {
		clog := models.NewChannelLogForSend(msg, h.RedactValues(ch))
		return h.Send(ctx, msg, &channels.SendResult{}, clog)
	}

	// no subscriber on the socket yet, so the publish is dropped but the send still succeeds
	require.NoError(t, send())
	assert.Empty(t, testsuite.CentrifugoHistory(t, rt, socket))

	// mark the socket subscribed (as the subscribe proxy endpoint would)
	vc := rt.VK.Get()
	_, err := vc.Do("SET", centrifugo.SubscriptionKey(socket), "1")
	vc.Close()
	require.NoError(t, err)

	require.NoError(t, send())

	sent := testsuite.CentrifugoHistory(t, rt, socket)
	require.Len(t, sent, 1)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sent[0], &decoded))
	assert.True(t, uuids.Is(decoded["uuid"].(string)))
	assert.Equal(t, "msg_out", decoded["type"])
	assert.Equal(t, "2025-10-13T11:20:30Z", decoded["created_on"])
	assert.Equal(t, "0191e180-7d60-7000-aded-7d8b151cbd5b", decoded["msg_uuid"])
	assert.Equal(t, "Hello there", decoded["text"])

	// a publish failure is returned as a send error
	rt.Centrifugo.Client.(*centrifugo.MockClient).SetError(errors.New("boom"))
	assert.EqualError(t, send(), "error publishing message event: boom")
}
