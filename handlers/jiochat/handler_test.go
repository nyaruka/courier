package jiochat

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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", map[string]any{configAppSecret: "secret123", configAppID: "app-id"}),
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
	{
		Label:                "Receive Message",
		URL:                  receiveURL,
		Data:                 validMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Simple Message"),
		ExpectedURN:          "jiochat:1234",
		ExpectedExternalID:   "123456",
		ExpectedDate:         time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC),
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid jiochat id",
	},
	{
		Label:                "Missing params",
		URL:                  receiveURL,
		Data:                 missingParamsRequired,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error:Field validation",
	},
	{
		Label:                "Missing params Event or MsgId",
		URL:                  receiveURL,
		Data:                 missingParams,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing parameters, must have either 'MsgId' or 'Event'",
	},
	{
		Label:                "Receive Image",
		URL:                  receiveURL,
		Data:                 imageMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "jiochat:1234",
		ExpectedExternalID:   "123456",
		ExpectedAttachments:  []string{"https://channels.jiochat.com/media/download.action?media_id=12"},
		ExpectedDate:         time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC),
	},
	{
		Label:                "Subscribe Event",
		URL:                  receiveURL,
		Data:                 subscribeEvent,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Event Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "jiochat:1234"},
		},
	},
	{
		Label:                "Unsubscribe Event",
		URL:                  receiveURL,
		Data:                 unsubscribeEvent,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unknown event",
	},
	{
		Label:                "Verify URL",
		URL:                  verifyURL,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "SUCCESS",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Verify URL Invalid signature",
		URL:                  verifyURL,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown request",
		PrepRequest:          addInvalidSignature,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// mocks the call to the Jiochat API
func buildMockJCAPI(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")
		defer r.Body.Close()

		if authorizationHeader != "Bearer ACCESS_TOKEN" {
			http.Error(w, "invalid file", http.StatusForbidden)
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
	logger := slog.Default()
	log.SetOutput(io.Discard)
	config := courier.NewConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return courier.NewServerWithLogger(config, backend, logger)
}

func TestDescribeURN(t *testing.T) {
	JCAPI := buildMockJCAPI(testCases)
	defer JCAPI.Close()

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
		{"jiochat:1337", map[string]string{"name": "John Doe"}},
		{"jiochat:4567", map[string]string{"name": ""}},
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
	sendURL = "https://channels.jiochat.com"

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://channels.jiochat.com/auth/token.action": {
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
	req, err := handler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=12", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=12", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))

	// and that we have a log for that request
	assert.Len(t, clog.HTTPLogs(), 1)
	assert.Equal(t, "https://channels.jiochat.com/auth/token.action", clog.HTTPLogs()[0].URL)

	// check that another request reads token from cache
	req, err = handler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=13", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=13", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))
	assert.Len(t, clog.HTTPLogs(), 1)

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "jiochat:12345",
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
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", map[string]any{configAppSecret: "secret123", configAppID: "app-id"})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"secret123"}, setupBackend)
}
