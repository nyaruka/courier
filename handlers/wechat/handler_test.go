package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US",
		map[string]any{courier.ConfigSecret: "secret123", configAppSecret: "app-secret123", configAppID: "app-id"}),
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

var testCases = []IncomingTestCase{
	{Label: "Receive Message", URL: receiveURL, Data: validMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "",
		ExpectedMsgText: Sp("Simple Message"), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
		ExpectedDate: time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, ExpectedRespStatus: 400, ExpectedBodyContains: "Error:Field validation"},
	{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, ExpectedRespStatus: 400, ExpectedBodyContains: "missing parameters, must have either 'MsgId' or 'Event'"},

	{Label: "Receive Image", URL: receiveURL, Data: imageMessage, ExpectedRespStatus: 200, ExpectedBodyContains: "",
		ExpectedMsgText: Sp(""), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
		ExpectedAttachments: []string{"https://api.weixin.qq.com/cgi-bin/media/get?media_id=12"},
		ExpectedDate:        time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{
		Label:                "Subscribe Event",
		URL:                  receiveURL,
		Data:                 subscribeEvent,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Event Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "wechat:1234"},
		},
	},

	{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, ExpectedRespStatus: 200, ExpectedBodyContains: "unknown event"},

	{Label: "Verify URL", URL: receiveURL, ExpectedRespStatus: 200, ExpectedBodyContains: "SUCCESS",
		PrepRequest: addValidSignature},

	{Label: "Verify URL Invalid signature", URL: receiveURL, ExpectedRespStatus: 400, ExpectedBodyContains: "unknown request",
		PrepRequest: addInvalidSignature},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// mocks the call to the WeChat API
func buildMockWCAPI(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

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

func newServer(backend courier.Backend) courier.Server {
	// for benchmarks, log to null
	logger := slog.Default()
	log.SetOutput(io.Discard)
	config := courier.NewConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return courier.NewServerWithLogger(config, backend, logger)
}

func TestDescribeURN(t *testing.T) {
	WCAPI := buildMockWCAPI(testCases)
	defer WCAPI.Close()

	mb := test.NewMockBackend()

	// ensure there's a cached access token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")

	s := newServer(mb)
	handler := newHandler().(*handler)
	handler.Initialize(s)
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, testChannels[0], handler.RedactValues(testChannels[0]))

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
	mb := test.NewMockBackend()

	// reset send URL
	sendURL = "https://api.weixin.qq.com/cgi-bin"

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.weixin.qq.com/cgi-bin/token?appid=app-id&grant_type=client_credential&secret=app-secret123": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"access_token": "SESAME"}`)),
		},
	}))

	// ensure that we start with no cached token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("DEL", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab")

	s := newServer(mb)
	handler := newHandler().(*handler)
	handler.Initialize(s)
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, testChannels[0], handler.RedactValues(testChannels[0]))

	// check that request has the fetched access token
	req, err := handler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://api.weixin.qq.com/cgi-bin/media/download.action?media_id=12", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/media/download.action?access_token=SESAME&media_id=12", req.URL.String())

	// and that we have a log for that request
	assert.Len(t, clog.HTTPLogs(), 1)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/token?appid=app-id&grant_type=client_credential&secret=**********", clog.HTTPLogs()[0].URL)

	// check that another request reads token from cache
	req, err = handler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://api.weixin.qq.com/cgi-bin/media/download.action?media_id=13", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://api.weixin.qq.com/cgi-bin/media/download.action?access_token=SESAME&media_id=13", req.URL.String())
	assert.Len(t, clog.HTTPLogs(), 1)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "wechat:12345",
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"Simple Message ☺"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "wechat:12345",
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"I need to keep adding more things to make it work"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "wechat:12345",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"My pic!\nhttps://foo.bar/image.jpg"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "wechat:12345",
		MockResponseStatus: 401,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func setupBackend(mb *test.MockBackend) {
	// ensure there's a cached access token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US", map[string]any{configAppSecret: "secret123", configAppID: "app-id"})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"secret123"}, setupBackend)
}
