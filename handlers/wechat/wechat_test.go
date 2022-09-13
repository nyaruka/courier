package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
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
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US",
		map[string]interface{}{courier.ConfigSecret: "secret", configAppSecret: "app-secret", configAppID: "app-id"}),
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

	stringSlice := []string{"secret", timestamp, nonce}
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

	stringSlice := []string{"secret", timestamp, nonce}
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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: receiveURL, Data: validMsg, ExpectedRespStatus: 200, ExpectedRespBody: "",
		ExpectedMsgText: Sp("Simple Message"), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
		ExpectedDate: time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, ExpectedRespStatus: 400, ExpectedRespBody: "Error:Field validation"},
	{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, ExpectedRespStatus: 400, ExpectedRespBody: "missing parameters, must have either 'MsgId' or 'Event'"},

	{Label: "Receive Image", URL: receiveURL, Data: imageMessage, ExpectedRespStatus: 200, ExpectedRespBody: "",
		ExpectedMsgText: Sp(""), ExpectedURN: "wechat:1234", ExpectedExternalID: "123456",
		ExpectedAttachments: []string{"https://api.weixin.qq.com/cgi-bin/media/get?media_id=12"},
		ExpectedDate:        time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{Label: "Subscribe Event", URL: receiveURL, Data: subscribeEvent, ExpectedRespStatus: 200, ExpectedRespBody: "Event Accepted",
		ExpectedEvent: courier.NewConversation, ExpectedURN: "wechat:1234"},

	{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, ExpectedRespStatus: 200, ExpectedRespBody: "unknown event"},

	{Label: "Verify URL", URL: receiveURL, ExpectedRespStatus: 200, ExpectedRespBody: "SUCCESS",
		PrepRequest: addValidSignature},

	{Label: "Verify URL Invalid signature", URL: receiveURL, ExpectedRespStatus: 400, ExpectedRespBody: "unknown request",
		PrepRequest: addInvalidSignature},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func TestFetchAccessToken(t *testing.T) {
	fetchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "token") {
			defer r.Body.Close()
			// valid token
			w.Write([]byte(`{"access_token": "TOKEN"}`))
		}

		// mark that we were called
		fetchCalled = true
	}))
	sendURL = server.URL
	fetchTimeout = time.Millisecond

	RunChannelTestCases(t, testChannels, newHandler(), []ChannelHandleTestCase{
		{
			Label:              "Receive Message",
			URL:                receiveURL,
			Data:               validMsg,
			ExpectedRespStatus: 200,
			ExpectedRespBody:   "",
			ExpectedMsgText:    Sp("Simple Message"),
			ExpectedURN:        "wechat:1234",
		},
		{
			Label:              "Verify URL",
			URL:                receiveURL,
			ExpectedRespStatus: 200,
			ExpectedRespBody:   "SUCCESS",
			PrepRequest:        addValidSignature,
		},
		{
			Label:              "Verify URL Invalid signature",
			URL:                receiveURL,
			ExpectedRespStatus: 400,
			ExpectedRespBody:   "unknown request",
			PrepRequest:        addInvalidSignature,
		},
	})

	// wait for our fetch to be called
	time.Sleep(100 * time.Millisecond)

	if !fetchCalled {
		t.Error("fetch access point should have been called")
	}

}

// mocks the call to the WeChat API
func buildMockWCAPI(testCases []ChannelHandleTestCase) *httptest.Server {
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
	logger := logrus.New()
	logger.Out = io.Discard
	logrus.SetOutput(io.Discard)
	config := courier.NewConfig()
	config.DB = "postgres://courier:courier@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return courier.NewServerWithLogger(config, backend, logger)
}

func TestDescribeURN(t *testing.T) {
	WCAPI := buildMockWCAPI(testCases)
	defer WCAPI.Close()

	mb := test.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()

	s := newServer(mb)
	handler := &handler{NewBaseHandler(courier.ChannelType("WC"), "WeChat")}
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

	AssertChannelLogRedaction(t, clog, []string{"secret"})
}

func TestBuildMediaRequest(t *testing.T) {
	mb := test.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
	s := newServer(mb)
	handler := &handler{NewBaseHandler(courier.ChannelType("WC"), "WeChat")}
	handler.Initialize(s)

	tcs := []struct {
		url string
	}{
		{
			fmt.Sprintf("%s/media/get?media_id=12", sendURL),
		},
	}

	for _, tc := range tcs {
		req, _ := handler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], tc.url)
		assert.Equal(t, fmt.Sprintf("%s/media/get?access_token=ACCESS_TOKEN&media_id=12", sendURL), req.URL.String())
	}

}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
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
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US", map[string]interface{}{configAppSecret: "secret", configAppID: "app-id"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"secret"}, setupBackend)
}
