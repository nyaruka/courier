package jiochat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/nyaruka/gocommon/urns"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", map[string]interface{}{courier.ConfigSecret: "secret"}),
}

var (
	receiveURL = "/c/jc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/rcv/msg/message"
	verifyURL  = "/c/jc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/"

	validMsg = `
	{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1454119029,
		"MsgType": "text",
		"MsgId": 123456,
		"Content": "Simple Message"
	}`

	subscribeEvent = `{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1454119029,
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
		"CreateTime": 1454119029,
		"MsgType": "text",
		"MsgId": 123456,
		"Content": "Simple Message"
	}`

	missingParams = `
	{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1454119029,
		"MsgType": "text",
		"Content": "Simple Message"
	}`

	imageMessage = `{
		"ToUsername": "12121212121212",
		"FromUserName": "1234",
		"CreateTime": 1454119029,
		"MsgType": "image",
		"MsgId": 123456,
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

	fmt.Println(signatureCheck)
	fmt.Println(r.URL)

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

	fmt.Println(signatureCheck)
	fmt.Println(r.URL)

	query := url.Values{}
	query.Set("signature", signatureCheck)
	query.Set("timestamp", timestamp)
	query.Set("nonce", "other")
	query.Set("echostr", "SUCCESS")

	r.URL.RawQuery = query.Encode()
}

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: receiveURL, Data: validMsg, Status: 200, Response: "Accepted",
		Text: Sp("Simple Message"), URN: Sp("jiochat:1234"), ExternalID: Sp("123456"),
		Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},

	{Label: "Missing params", URL: receiveURL, Data: missingParamsRequired, Status: 400, Response: "Error:Field validation"},
	{Label: "Missing params Event or MsgId", URL: receiveURL, Data: missingParams, Status: 400, Response: "missing parameters, must have either 'MsgId' or 'Event'"},

	{Label: "Receive Image", URL: receiveURL, Data: imageMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("jiochat:1234"), ExternalID: Sp("123456"),
		Attachment: Sp("https://channels.jiochat.com/media/download.action?media_id=12"),
		Date:       Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},

	{Label: "Subscribe Event", URL: receiveURL, Data: subscribeEvent, Status: 200, Response: "Event Accepted",
		ChannelEvent: Sp(courier.NewConversation), URN: Sp("jiochat:1234")},

	{Label: "Unsubscribe Event", URL: receiveURL, Data: unsubscribeEvent, Status: 400, Response: "unknown event"},

	{Label: "Verify URL", URL: verifyURL, Status: 200, Response: "SUCCESS",
		PrepRequest: addValidSignature},

	{Label: "Verify URL Invalid signature", URL: verifyURL, Status: 400, Response: "unknown request",
		PrepRequest: addInvalidSignature},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// mocks the call to the Jiochat API
func buildMockJCAPI(testCases []ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openID := r.URL.Query().Get("openid")
		defer r.Body.Close()

		// user has a name
		if strings.HasSuffix(openID, "1337") {
			w.Write([]byte(`{ "nickname": "John Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "nickname": ""}`))
	}))
	userDetailsURL = server.URL

	return server
}

func TestDescribe(t *testing.T) {
	JCAPI := buildMockJCAPI(testCases)
	defer JCAPI.Close()

	handler := newHandler().(courier.URNDescriber)
	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{
		{"jiochat:1337", map[string]string{"name": "John Doe"}},
		{"jiochat:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}
