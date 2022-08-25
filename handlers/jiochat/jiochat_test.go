package jiochat

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
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", map[string]interface{}{configAppSecret: "secret", configAppID: "app-id"}),
}

var (
	receiveURL = "/c/jc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/rcv/msg/message"
	verifyURL  = "/c/jc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/"

	validMsg = `
	{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1518774424438,
		"MsgType": "text",
		"MsgId": "123456",
		"Content": "Simple Message"
	}`

	invalidURN = `
	{
		"ToUsername": "1212121221212",
		"FromUserName": "1234abc",
		"CreateTime": 1518774424438,
		"MsgType": "text",
		"MsgId": "123456",
		"Content": "Simple Message"
	}`

	subscribeEvent = `{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1518774424438,
		"MsgType": "event",
		"Event": "subscribe"
	}`

	unsubscribeEvent = `{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1454119029,
		"MsgType": "event",
		"Event": "unsubscribe"
	}`

	missingParamsRequired = `
	{
		"ToUsername": "12121212121212",
		"CreateTime": 1518774424438,
		"MsgType": "text",
		"MsgId": "123456",
		"Content": "Simple Message"
	}`

	missingParams = `
	{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1518774424438,
		"MsgType": "text",
		"Content": "Simple Message"
	}`

	imageMessage = `{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1518774424438,
		"MsgType": "image",
		"MsgId": "123456",
		"MediaId": "12"
	}`
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
	{Label: "Receive Message", URL: receiveURL, Data: validMsg, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Simple Message"), ExpectedURN: Sp("jiochat:1234"), ExpectedExternalID: Sp("123456"),
		ExpectedDate: time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, ExpectedStatus: 400, ExpectedResponse: "invalid jiochat id"},
	{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, ExpectedStatus: 400, ExpectedResponse: "Error:Field validation"},
	{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, ExpectedStatus: 400, ExpectedResponse: "missing parameters, must have either 'MsgId' or 'Event'"},

	{Label: "Receive Image", URL: receiveURL, Data: imageMessage, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp(""), ExpectedURN: Sp("jiochat:1234"), ExpectedExternalID: Sp("123456"),
		ExpectedAttachments: []string{"https://channels.jiochat.com/media/download.action?media_id=12"},
		ExpectedDate:        time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC)},

	{Label: "Subscribe Event", URL: receiveURL, Data: subscribeEvent, ExpectedStatus: 200, ExpectedResponse: "Event Accepted",
		ExpectedChannelEvent: courier.NewConversation, ExpectedURN: Sp("jiochat:1234")},

	{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, ExpectedStatus: 200, ExpectedResponse: "unknown event"},

	{Label: "Verify URL", URL: verifyURL, ExpectedStatus: 200, ExpectedResponse: "SUCCESS",
		PrepRequest: addValidSignature},

	{Label: "Verify URL Invalid signature", URL: verifyURL, ExpectedStatus: 400, ExpectedResponse: "unknown request",
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
		if strings.HasSuffix(r.URL.Path, "auth/token.action") {
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
		{Label: "Receive Message", URL: receiveURL, Data: validMsg, ExpectedStatus: 200, ExpectedResponse: "Accepted"},

		{Label: "Verify URL", URL: verifyURL, ExpectedStatus: 200, ExpectedResponse: "SUCCESS",
			PrepRequest: addValidSignature},

		{Label: "Verify URL Invalid signature", URL: verifyURL, ExpectedStatus: 400, ExpectedResponse: "unknown request",
			PrepRequest: addInvalidSignature},
	})

	// wait for our fetch to be called
	time.Sleep(100 * time.Millisecond)

	if !fetchCalled {
		t.Error("fetch access point should have been called")
	}

}

// mocks the call to the Jiochat API
func buildMockJCAPI(testCases []ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")
		defer r.Body.Close()

		if authorizationHeader != "Bearer ACCESS_TOKEN" {
			http.Error(w, "invalid file", 403)
			return
		}

		if strings.HasSuffix(r.URL.Path, "user/info.action") {
			openID := r.URL.Query().Get("openid")

			// user has a name
			if strings.HasSuffix(openID, "1337") {
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
	config.DB = "postgres://courier:courier@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return courier.NewServerWithLogger(config, backend, logger)
}

func TestDescribeURN(t *testing.T) {
	JCAPI := buildMockJCAPI(testCases)
	defer JCAPI.Close()

	mb := test.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "jiochat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()

	s := newServer(mb)
	handler := &handler{handlers.NewBaseHandler(courier.ChannelType("JC"), "Jiochat")}
	handler.Initialize(s)
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, testChannels[0])

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"jiochat:1337", map[string]string{"name": "John Doe"}},
		{"jiochat:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}
}

func TestBuildMediaRequest(t *testing.T) {
	mb := test.NewMockBackend()
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "jiochat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
	s := newServer(mb)
	handler := &handler{handlers.NewBaseHandler(courier.ChannelType("JC"), "Jiochat")}
	handler.Initialize(s)

	tcs := []struct {
		url                 string
		authorizationHeader string
	}{
		{
			fmt.Sprintf("%s/media/download.action?media_id=12", sendURL),
			"Bearer ACCESS_TOKEN",
		},
	}

	for _, tc := range tcs {
		req, _ := handler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], tc.url)
		assert.Equal(t, tc.url, req.URL.String())
		assert.Equal(t, tc.authorizationHeader, req.Header.Get("Authorization"))
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
		MsgURN:             "jiochat:12345",
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"Simple Message ☺"}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "jiochat:12345",
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"I need to keep adding more things to make it work"}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "jiochat:12345",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"msgtype":"text","touser":"12345","text":{"content":"My pic!\nhttps://foo.bar/image.jpg"}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "jiochat:12345",
		MockResponseStatus: 401,
		ExpectedStatus:     "E",
		SendPrep:           setSendURL,
	},
}

func setupBackend(mb *test.MockBackend) {
	conn := mb.RedisPool().Get()

	_, err := conn.Do("SET", "jiochat_channel_access_token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
	if err != nil {
		log.Fatal(err)
	}

	conn.Close()
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", map[string]interface{}{configAppSecret: "secret", configAppID: "app-id"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, setupBackend)
}
