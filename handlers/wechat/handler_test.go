package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US",
		[]string{urns.WeChat.Prefix},
		map[string]any{models.ConfigSecret: "secret123", configAppSecret: "app-secret123", configAppID: "app-id"}),
}

var (
	receiveURL = "/c/wc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab"

	validMsg = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <FromUserName><![CDATA[1234]]></FromUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[text]]></MsgType>
    <Content><![CDATA[Simple Message]]></Content>
    <MsgId>123456</MsgId>
	</xml>
	`

	subscribeEvent = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <FromUserName><![CDATA[1234]]></FromUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[event]]></MsgType>
    <Event><![CDATA[subscribe]]></Event>
	</xml>`

	unsubscribeEvent = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <FromUserName><![CDATA[1234]]></FromUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[event]]></MsgType>
    <Event><![CDATA[unsubscribe]]></Event>
	</xml>`

	missingParamsRequired = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[text]]></MsgType>
    <Content><![CDATA[Simple Message]]></Content>
    <MsgId>123456</MsgId>
	</xml>
	`

	missingParams = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <FromUserName><![CDATA[1234]]></FromUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[text]]></MsgType>
    <Content><![CDATA[Simple Message]]></Content>
	</xml>
	`

	imageMessage = `
	<xml>
    <ToUserName><![CDATA[12121212121212]]></ToUserName>
    <FromUserName><![CDATA[1234]]></FromUserName>
    <CreateTime>1518774424438</CreateTime>
    <MsgType><![CDATA[image]]></MsgType>
	<MsgId>123456</MsgId>
	<MediaId>12</MediaId>
	</xml>
	`
)

func addValidSignature(r *http.Request) {
	t := time.Now()
	timestamp := t.Format("20060102150405")
	nonce := "nonce"

	stringSlice := []string{"secret123", timestamp, nonce}
	sort.Strings(stringSlice)

	value := strings.Join(stringSlice, "")

	hashObject := sha1.New()
	hashObject.Write([]byte(value))
	signatureCheck := hex.EncodeToString(hashObject.Sum(nil))

	query := url.Values{}
	query.Set("signature", signatureCheck)
	query.Set("timestamp", timestamp)
	query.Set("nonce", nonce)
	query.Set("echostr", "SUCCESS")

	r.URL.RawQuery = query.Encode()

}

func addInvalidSignature(r *http.Request) {
	t := time.Now()
	timestamp := t.Format("20060102150405")
	nonce := "nonce"

	stringSlice := []string{"secret123", timestamp, nonce}
	sort.Strings(stringSlice)

	value := strings.Join(stringSlice, "")

	hashObject := sha1.New()
	hashObject.Write([]byte(value))
	signatureCheck := hex.EncodeToString(hashObject.Sum(nil))

	query := url.Values{}
	query.Set("signature", signatureCheck)
	query.Set("timestamp", timestamp)
	query.Set("nonce", "other")
	query.Set("echostr", "SUCCESS")

	r.URL.RawQuery = query.Encode()
}

// built as a function because the expected attachment URL depends on the API URL, which incoming tests
// repoint at a mock server
func incomingCases() []IncomingTestCase {
	return []IncomingTestCase{
		{Label: "Receive Message", URL: receiveURL, Data: validMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "",
			ExpectedMsgText: Sp("Simple Message"), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
			ExpectedDate: time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

		{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, ExpectedRespStatus: 400, ExpectedBodyContains: "Error:Field validation"},
		{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, ExpectedRespStatus: 400, ExpectedBodyContains: "missing parameters, must have either 'MsgId' or 'Event'"},

		{Label: "Receive Image", URL: receiveURL, Data: imageMessage, ExpectedRespStatus: 200, ExpectedBodyContains: "",
			ExpectedMsgText: Sp(""), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
			ExpectedAttachments: []string{fmt.Sprintf("%s/media/get?media_id=12", sendURL)},
			ExpectedDate:        time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

		{
			Label:                "Subscribe Event",
			URL:                  receiveURL,
			Data:                 subscribeEvent,
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "Event Accepted",
			ExpectedEvents: []ExpectedEvent{
				{Type: models.EventTypeNewConversation, URN: "wechat:1234"},
			},
		},

		{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, ExpectedRespStatus: 200, ExpectedBodyContains: "unknown event"},

		{Label: "Verify URL", URL: receiveURL, ExpectedRespStatus: 200, ExpectedBodyContains: "SUCCESS",
			PrepRequest: addValidSignature},

		{Label: "Verify URL Invalid signature", URL: receiveURL, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown request",
			PrepRequest: addInvalidSignature},
	}
}

func TestIncoming(t *testing.T) {
	// creating a contact for an incoming message looks up their name via the API, so point that at a mock
	defer func(u string) { sendURL = u }(sendURL)
	WCAPI := buildMockWCAPI()
	defer WCAPI.Close()

	RunIncomingTestCases(t, testChannels, newHandler(), incomingCases())
}

// mocks the call to the WeChat API
func buildMockWCAPI() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// a request for an access token is the one request that doesn't carry one
		if strings.HasSuffix(r.URL.Path, "/token") {
			w.Write([]byte(`{"access_token": "ACCESS_TOKEN"}`))
			return
		}

		if accessToken != "ACCESS_TOKEN" {
			http.Error(w, "invalid file", http.StatusForbidden)
			return
		}

		if strings.HasSuffix(r.URL.Path, "user/info") {
			openID := r.URL.Query().Get("openid")

			// user has a name
			if strings.HasSuffix(openID, "KNOWN_OPEN_ID") {
				w.Write([]byte(`{ "nickname": "John Doe"}`))
				return
			}

			// no name
			w.Write([]byte(`{ "nickname": ""}`))

		}

	}))
	sendURL = server.URL

	return server
}

func TestDescribeURN(t *testing.T) {
	defer func(u string) { sendURL = u }(sendURL)
	WCAPI := buildMockWCAPI()
	defer WCAPI.Close()

	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)

	// use a plain client so the handler can reach the mock API on localhost
	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")

	s := web.NewServer(rt)
	handler := newHandler().(*handler)
	s.MountHandler(handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"wechat:abcdeKNOWN_OPEN_ID", map[string]string{"name": "John Doe"}},
		{"wechat:foo__NOT__KNOWN", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

func TestBuildAttachmentRequest(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	// reset send URL
	sendURL = "https://api.weixin.qq.com/cgi-bin"

	// ensure that we start with no cached token
	testsuite.ResetValkey(t, rt)

	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	s := web.NewServer(rt)
	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.weixin.qq.com/cgi-bin/token?appid=app-id&grant_type=client_credential&secret=app-secret123": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"access_token": "SESAME"}`)),
		},
	})
	handler := newHandler().(*handler)
	s.MountHandler(handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	// check that request has the fetched access token
	req, err := handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://api.weixin.qq.com/cgi-bin/media/download.action?media_id=12", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/media/download.action?access_token=SESAME&media_id=12", req.URL.String())

	// and that we have a log for that request
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/token?appid=app-id&grant_type=client_credential&secret=**********", clog.HttpLogs[0].URL)

	// check that another request reads token from cache
	req, err = handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://api.weixin.qq.com/cgi-bin/media/download.action?media_id=13", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/media/download.action?access_token=SESAME&media_id=13", req.URL.String())
	assert.Len(t, clog.HttpLogs, 1)
}

func TestFetchAccessTokenThrottled(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	// reset send URL
	sendURL = "https://api.weixin.qq.com/cgi-bin"

	// ensure that we start with no cached token
	testsuite.ResetValkey(t, rt)

	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	s := web.NewServer(rt)
	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.weixin.qq.com/cgi-bin/token?appid=app-id&grant_type=client_credential&secret=app-secret123": {
			httpx.NewMockResponse(429, nil, []byte(`{"errcode": 45009, "errmsg": "reach max api daily quota limit"}`)),
		},
	})
	handler := newHandler().(*handler)
	s.MountHandler(handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	// a rate limited token fetch is throttling rather than an empty token
	_, _, err := handler.fetchAccessToken(testChannels[0], clog)
	assert.Equal(t, channels.ErrConnectionThrottled, err)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "wechat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
			},
			Body: `{"msgtype":"text","touser":"12345","text":{"content":"Simple Message ☺"}}`,
		}},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "wechat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(200, nil, []byte(``)),
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Accept":       "application/json",
				},
				Body: `{"msgtype":"text","touser":"12345","text":{"content":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}}`,
			},
			{
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Accept":       "application/json",
				},
				Body: `{"msgtype":"text","touser":"12345","text":{"content":"I need to keep adding more things to make it work"}}`,
			},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "wechat:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
			},
			Body: `{"msgtype":"text","touser":"12345","text":{"content":"My pic!\nhttps://foo.bar/image.jpg"}}`,
		}},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "wechat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(401, nil, []byte(`Error`)),
			},
		},
	},
	{
		Label:   "Connection Error",
		MsgText: "Error Message",
		MsgURN:  "wechat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(500, nil, []byte(`Error`)),
			},
		},
		ExpectedError: channels.ErrConnectionFailed,
	},
	{
		Label:   "Throttled",
		MsgText: "Error Message",
		MsgURN:  "wechat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.weixin.qq.com/cgi-bin/message/custom/send*": {
				httpx.NewMockResponse(429, nil, []byte(`Error`)),
			},
		},
		ExpectedError: channels.ErrConnectionThrottled,
	},
}

func setupBackend(t *testing.T, rt *runtime.Runtime) {
	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US", []string{urns.WeChat.Prefix}, map[string]any{configAppSecret: "secret123", configAppID: "app-id"})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"secret123"}, setupBackend)
}

func TestSendEvent(t *testing.T) {
	// other tests repoint sendURL at mock servers, so pin it for this test
	defer func(u string) { sendURL = u }(sendURL)
	sendURL = "https://api.weixin.qq.com/cgi-bin"

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US", []string{urns.WeChat.Prefix}, map[string]any{configAppSecret: "secret123", configAppID: "app-id"})

	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)

	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")

	s := web.NewServer(rt)
	h := newHandler().(*handler)
	s.MountHandler(h)

	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.weixin.qq.com/cgi-bin/message/custom/typing*": {
			httpx.NewMockResponse(200, nil, []byte(`{"errcode": 0, "errmsg": "ok"}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"errcode": 0, "errmsg": "ok"}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"errcode": 45015, "errmsg": "response out of time limit"}`)),
			httpx.NewMockResponse(400, nil, []byte(`bad request`)),
			httpx.MockConnectionError,
		},
	})

	// typing events are supported including explicit stop
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 12 * time.Second, events.TypeTypingStopped: 0}, h.SendableEvents(ch))

	channelRef := assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WeChat")
	typing := events.NewTypingStarted(events.DirectionOutgoing, channelRef, "wechat:abcde12345", "")

	// a typing started event is sent as a Typing command
	clog := models.NewChannelLogForEventSend(ch, nil)
	err := h.SendEvent(context.Background(), ch, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Contains(t, clog.HttpLogs[0].URL, "https://api.weixin.qq.com/cgi-bin/message/custom/typing")
	assert.Contains(t, clog.HttpLogs[0].Request, `{"touser":"abcde12345","command":"Typing"}`)

	// and a typing stopped event as a CancelTyping command
	err = h.SendEvent(context.Background(), ch, events.NewTypingStopped(events.DirectionOutgoing, channelRef, "wechat:abcde12345", ""), clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 2)
	assert.Contains(t, clog.HttpLogs[1].Request, `{"touser":"abcde12345","command":"CancelTyping"}`)

	// a WeChat error in a 200 response is a response error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a non-2XX response
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// and a connection error is a connection error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// an event type the handler doesn't declare support for can't be sent
	err = h.SendEvent(context.Background(), ch, events.NewContactLanguageChanged("eng"), clog)
	assert.ErrorContains(t, err, "unsupported event type: contact_language_changed")
}
