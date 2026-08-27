package webchat

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
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

// webchat traffic is internal to the platform so its channel logs are never stored - each visitor request
// would otherwise write one
func TestChannelLogsNotStored(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	random.SetSecureSource(random.NewSeededSource(1234))
	defer random.SetSecureSource(random.DefaultSecureSource)

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	require.NoError(t, s.MountHandler(newHandler()))

	// capture the in-memory logs of handled requests
	var clogs []*models.ChannelLog
	s.OnRequestHandled(func(ch *models.Channel, evts []channels.Event, clog *models.ChannelLog) { clogs = append(clogs, clog) })

	post := func(path, body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(body))
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	assert.Equal(t, 200, post(startURL, `{}`).Code)
	assert.Equal(t, 200, post(receiveURL, `{"chat_id": "`+testChatID+`", "text": "Hello"}`).Code)

	// the logs still exist in memory during handling...
	require.Len(t, clogs, 2)
	assert.Equal(t, models.ChannelLogTypeChatStart, clogs[0].Type)
	assert.Equal(t, models.ChannelLogTypeMsgReceive, clogs[1].Type)

	// ...but are never written to storage
	rt.Dynamo.Main.Flush()
	for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
		assert.False(t, strings.HasPrefix(item.Key.SK, "log#"), "unexpected channel log stored: %s", item.Key.SK)
	}
}

func TestStartRateLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	const otherChannelUUID = "b81c3f45-2d6e-4a1f-9c72-8e5d0a4b6f13"

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	testsuite.InsertChannel(t, rt, test.NewMockChannel(otherChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix}, nil))
	require.NoError(t, s.MountHandler(newHandler()))

	startOn := func(chURL, ip string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+chURL, strings.NewReader(`{}`))
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}
	start := func(ip string) *httptest.ResponseRecorder { return startOn(startURL, ip) }

	// an IP can start up to the limit of chats within the window...
	for i := range startLimit {
		assert.Equal(t, 200, start("41.23.45.67:1234").Code, "start %d", i)
	}

	// ...then gets throttled, with the CORS header still on the error so the widget can read it
	rr := start("41.23.45.67:1234")
	assert.Equal(t, 429, rr.Code)
	assert.Contains(t, rr.Body.String(), "rate limit exceeded")
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// without a contact being created
	var contacts int
	require.NoError(t, rt.DB.Get(&contacts, `SELECT count(*) FROM contacts_contact WHERE uuid != 'a984069d-0008-4d8c-a772-b14a8a6acccc'`))
	assert.Equal(t, startLimit, contacts)

	// but other IPs aren't affected
	assert.Equal(t, 200, start("41.23.45.68:1234").Code)

	// nor is the same IP on a different channel - the limit is scoped per channel
	assert.Equal(t, 200, startOn("/c/wch/"+otherChannelUUID+"/start", "41.23.45.67:1234").Code)

	// and the count expires with the window
	vc := rt.VK.Get()
	defer vc.Close()
	ttl, err := redis.Int(vc.Do("TTL", "chat-starts:"+channelUUID+"|41.23.45.67"))
	require.NoError(t, err)
	assert.Greater(t, ttl, 0)
	assert.LessOrEqual(t, ttl, startLimitWindow)
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

func TestAllowedDomains(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	const cfgChannelUUID = "7d3fb8a2-5c1e-4b9f-a6d4-2e8c0f7b5a19"
	cfgStartURL := "/c/wch/" + cfgChannelUUID + "/start"
	cfgReceiveURL := "/c/wch/" + cfgChannelUUID + "/receive"

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	testsuite.InsertChannel(t, rt, test.NewMockChannel(cfgChannelUUID, "WCH", "", "", []string{urns.WebChat.Prefix},
		map[string]any{"allowed_domains": []string{"example.com", "localhost:3000"}},
	))
	require.NoError(t, s.MountHandler(newHandler()))

	request := func(path, origin string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(`{}`))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rr := httptest.NewRecorder()
		s.Router().ServeHTTP(rr, req)
		return rr
	}

	// a channel without allowed_domains ignores the origin and keeps the wildcard header
	rr := request(startURL, "https://anywhere.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	// an allowed origin gets itself reflected back instead of the wildcard, with Vary: Origin
	rr = request(cfgStartURL, "https://example.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))

	// host matching is case-insensitive, and entries can carry a port
	rr = request(cfgStartURL, "https://EXAMPLE.com")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "https://EXAMPLE.com", rr.Header().Get("Access-Control-Allow-Origin"))
	rr = request(cfgStartURL, "http://localhost:3000")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "http://localhost:3000", rr.Header().Get("Access-Control-Allow-Origin"))

	// a disallowed origin gets a 403 with no allow-origin header, so the embedding page can't read it either
	for _, origin := range []string{"https://evil.com", "https://sub.example.com", "https://example.com:8080", "null"} {
		rr = request(cfgStartURL, origin)
		assert.Equal(t, 403, rr.Code, origin)
		assert.Contains(t, rr.Body.String(), "origin not allowed", origin)
		assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"), origin)
	}

	// on the receive endpoint too
	rr = request(cfgReceiveURL, "https://evil.com")
	assert.Equal(t, 403, rr.Code)
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))

	// without any contacts being created by the blocked starts
	var contacts int
	require.NoError(t, rt.DB.Get(&contacts, `SELECT count(*) FROM contacts_contact WHERE uuid != 'a984069d-0008-4d8c-a772-b14a8a6acccc'`))
	assert.Equal(t, 4, contacts)

	// but a request with no Origin header (a non-browser client) passes, still marked as varying on origin
	rr = request(cfgStartURL, "")
	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", rr.Header().Get("Vary"))

	// and preflights stay permissive - the channel isn't loaded for them, and the POST response is what gates
	// the browser
	req, _ := http.NewRequest(http.MethodOptions, "https://localhost"+cfgStartURL, nil)
	req.Header.Set("Origin", "https://evil.com")
	rr = httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	assert.Equal(t, 204, rr.Code)
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
