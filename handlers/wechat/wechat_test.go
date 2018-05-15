package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US",
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
	sort.Sort(sort.StringSlice(stringSlice))

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
	sort.Sort(sort.StringSlice(stringSlice))

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
	{Label: "Receive Message", URL: receiveURL, Data: validMsg, Status: 200, Response: "",
		Text: Sp("Simple Message"), URN: Sp("wechat:1234"), ExternalID: Sp("123456"),
		Date: Tp(time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC))},

	{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, Status: 400, Response: "Error:Field validation"},
	{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, Status: 400, Response: "missing parameters, must have either 'MsgId' or 'Event'"},

	{Label: "Receive Image", URL: receiveURL, Data: imageMessage, Status: 200, Response: "",
		Text: Sp(""), URN: Sp("wechat:1234"), ExternalID: Sp("123456"),
		Attachment: Sp("https://api.weixin.qq.com/cgi-bin/media/get?media_id=12"),
		Date:       Tp(time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC))},

	{Label: "Subscribe Event", URL: receiveURL, Data: subscribeEvent, Status: 200, Response: "Event Accepted",
		ChannelEvent: Sp(courier.NewConversation), URN: Sp("wechat:1234")},

	{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, Status: 200, Response: "unknown event"},

	{Label: "Verify URL", URL: receiveURL, Status: 200, Response: "SUCCESS",
		PrepRequest: addValidSignature},

	{Label: "Verify URL Invalid signature", URL: receiveURL, Status: 400, Response: "unknown request",
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
		{Label: "Receive Message", URL: receiveURL, Data: validMsg, Status: 200, Response: ""},

		{Label: "Verify URL", URL: receiveURL, Status: 200, Response: "SUCCESS",
			PrepRequest: addValidSignature},

		{Label: "Verify URL Invalid signature", URL: receiveURL, Status: 400, Response: "unknown request",
			PrepRequest: addInvalidSignature},
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
			http.Error(w, "invalid file", 403)
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
	logger.Out = ioutil.Discard
	logrus.SetOutput(ioutil.Discard)
	config := courier.NewConfig()
	config.DB = "postgres://courier@localhost/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return courier.NewServerWithLogger(config, backend, logger)
}

func TestDescribe(t *testing.T) {
	WCAPI := buildMockWCAPI(testCases)
	defer WCAPI.Close()

	mb := courier.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("Set", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()

	s := newServer(mb)
	handler := &handler{handlers.NewBaseHandler(courier.ChannelType("WC"), "WeChat")}
	handler.Initialize(s)

	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{
		{"wechat:abcdeKNOWN_OPEN_ID", map[string]string{"name": "John Doe"}},
		{"wechat:foo__NOT__KNOWN", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}

func TestBuildMediaRequest(t *testing.T) {
	mb := courier.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("Set", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
	s := newServer(mb)
	handler := &handler{handlers.NewBaseHandler(courier.ChannelType("WC"), "WeChat")}
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
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "wechat:12345",
		Status:         "W",
		ExternalID:     "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"Simple Message ☺"}}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "wechat:12345",
		Status:         "W",
		ExternalID:     "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"I need to keep adding more things to make it work"}}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "wechat:12345",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "",
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"My pic!\nhttps://foo.bar/image.jpg"}}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "wechat:12345",
		Status:         "E",
		ResponseStatus: 401,
		Error:          "received non 200 status: 401",
		SendPrep:       setSendURL},
}

func setupBackend(mb *courier.MockBackend) {
	conn := mb.RedisPool().Get()

	_, err := conn.Do("Set", "wechat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WC", "2020", "US", map[string]interface{}{configAppSecret: "secret", configAppID: "app-id"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, setupBackend)
}
