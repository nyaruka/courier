package jiochat

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
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", []string{urns.JioChat.Prefix}, map[string]any{configAppSecret: "secret123", configAppID: "app-id"}),
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

// built as a function because the expected attachment URL depends on the API URL, which incoming tests
// repoint at a mock server
func incomingCases() []IncomingTestCase {
	return []IncomingTestCase{
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
			ExpectedAttachments:  []string{fmt.Sprintf("%s/media/download.action?media_id=12", sendURL)},
			ExpectedDate:         time.Date(2018, 2, 16, 9, 47, 4, 438000000, time.UTC),
		},
		{
			Label:                "Subscribe Event",
			URL:                  receiveURL,
			Data:                 subscribeEvent,
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "Event Accepted",
			ExpectedEvents: []ExpectedEvent{
				{Type: models.EventTypeNewConversation, URN: "jiochat:1234"},
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
}

func TestIncoming(t *testing.T) {
	// creating a contact for an incoming message looks up their name via the API, so point that at a mock
	defer func(u string) { sendURL = u }(sendURL)
	JCAPI := buildMockJCAPI()
	defer JCAPI.Close()

	RunIncomingTestCases(t, testChannels, newHandler(), incomingCases())
}

// mocks the call to the Jiochat API
func buildMockJCAPI() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")
		defer r.Body.Close()

		// a request for an access token is the one request that doesn't carry one
		if strings.HasSuffix(r.URL.Path, "auth/token.action") {
			w.Write([]byte(`{"access_token": "ACCESS_TOKEN"}`))
			return
		}

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

func TestDescribeURN(t *testing.T) {
	defer func(u string) { sendURL = u }(sendURL)
	JCAPI := buildMockJCAPI()
	defer JCAPI.Close()

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
	_, rt := testsuite.Runtime(t)

	// reset send URL
	sendURL = "https://channels.jiochat.com"

	// ensure that we start with no cached token
	testsuite.ResetValkey(t, rt)

	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	s := web.NewServer(rt)
	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://channels.jiochat.com/auth/token.action": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"access_token": "SESAME"}`)),
		},
	})
	handler := newHandler().(*handler)
	s.MountHandler(handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	// check that request has the fetched access token
	req, err := handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=12", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=12", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))

	// and that we have a log for that request
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://channels.jiochat.com/auth/token.action", clog.HttpLogs[0].URL)

	// check that another request reads token from cache
	req, err = handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=13", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=13", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))
	assert.Len(t, clog.HttpLogs, 1)

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "jiochat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://channels.jiochat.com/custom/custom_send.action": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"msgtype":"text","touser":"12345","text":{"content":"Simple Message ☺"}}`,
		}},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "jiochat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://channels.jiochat.com/custom/custom_send.action": {
				httpx.NewMockResponse(200, nil, []byte(``)),
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Bearer ACCESS_TOKEN",
				},
				Body: `{"msgtype":"text","touser":"12345","text":{"content":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}}`,
			},
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Bearer ACCESS_TOKEN",
				},
				Body: `{"msgtype":"text","touser":"12345","text":{"content":"I need to keep adding more things to make it work"}}`,
			},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "jiochat:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://channels.jiochat.com/custom/custom_send.action": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"msgtype":"text","touser":"12345","text":{"content":"My pic!\nhttps://foo.bar/image.jpg"}}`,
		}},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "jiochat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://channels.jiochat.com/custom/custom_send.action": {
				httpx.NewMockResponse(401, nil, []byte(``)),
			},
		},
		ExpectedError: channels.ErrResponseStatus,
	},
	{
		Label:   "Throttled",
		MsgText: "Error Message",
		MsgURN:  "jiochat:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://channels.jiochat.com/custom/custom_send.action": {
				httpx.NewMockResponse(429, nil, []byte(``)),
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
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", []string{urns.JioChat.Prefix}, map[string]any{configAppSecret: "secret123", configAppID: "app-id"})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"secret123"}, setupBackend)
}
