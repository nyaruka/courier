package instagram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8ab23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "1234", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var helloMsg = `{
	"object":"instagram",
	"entry": [{
	  "id": "1234",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "1234"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var duplicateMsg = `{
	"object":"instagram",
	"entry": [{
	  "id": "1234",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "1234"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	},
	{
	  "id": "1234",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "1234"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var invalidURN = `{
	"object":"instagram",
	"entry": [{
	  "id": "1234",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "1234"
			},
			"sender": {
			  "id": "abc5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var attachment = `{
	"object":"instagram",
	"entry": [{
	  	"id": "1234",
	  	"messaging": [{
				"message": {
		  			"mid": "external_id",
		  			"attachments":[{
      	      		"type":"image",
      	      		"payload":{
						"url":"https://image-url/foo.png"
						}
					}]
				},
				"recipient": {
					"id": "1234"
				},
				"sender": {
					"id": "5678"
				},
				"timestamp": 1459991487970
	    }],
	  	"time": 1459991487970
	}]
}`

var like_heart = `{
	"object":"instagram",
	"entry":[{
		"id":"1234",
		"messaging":[{
			"sender":{"id":"5678"},
			"recipient":{"id":"1234"},
			"timestamp":1459991487970,
			"message":{
				"mid":"external_id",
				"attachments":[{
					"type":"like_heart"
				}]
			}
		}],
		"time":1459991487970
	}]
}`

var differentPage = `{
	"object":"instagram",
	"entry": [{
	  "id": "1234",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "1235"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var echo = `{
	"object":"instagram",
	"entry": [{
		"id": "1234",
		"messaging": [{
			"recipient": {
				"id": "1234"
			},
			"sender": {
				"id": "5678"
			},
			"timestamp": 1459991487970,
			"message": {
				"is_echo": true,
				"mid": "qT7ywaK"
			}
		}]
	}]
}`

var icebreakerGetStarted = `{
	"object":"instagram",
	"entry": [{
	  "id": "1234",
	  "messaging": [{
			"postback": {
				"title": "icebreaker question",  
				"payload": "get_started"
			},
			"recipient": {
			  "id": "1234"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var notInstagram = `{
	"object":"notinstagram",
	"entry": [{}]
}`

var noEntries = `{
	"object":"instagram",
	"entry": []
}`

var noMessagingEntries = `{
	"object":"instagram",
	"entry": [{
		"id": "1234"
	}]
}`

var unkownMessagingEntry = `{
	"object":"instagram",
	"entry": [{
		"id": "1234",
		"messaging": [{
			"recipient": {
				"id": "1234"
			},
			"sender": {
				"id": "5678"
			},
			"timestamp": 1459991487970
		}]
	}]
}`

var notJSON = `blargh`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: "/c/ig/receive", Data: helloMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Invalid Signature", URL: "/c/ig/receive", Data: helloMsg, Status: 400, Response: "invalid request signature", PrepRequest: addInvalidSignature},

	{Label: "No Duplicate Receive Message", URL: "/c/ig/receive", Data: duplicateMsg, Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Attachment", URL: "/c/ig/receive", Data: attachment, Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Like Heart", URL: "/c/ig/receive", Data: like_heart, Status: 200, Response: "Handled",
		Text: Sp(""), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Icebreaker Get Started", URL: "/c/ig/receive", Data: icebreakerGetStarted, Status: 200, Response: "Handled",
		URN: Sp("instagram:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "icebreaker question", "payload": "get_started"},
		PrepRequest:       addValidSignature},

	{Label: "Different Page", URL: "/c/ig/receive", Data: differentPage, Status: 200, Response: `"data":[]`, PrepRequest: addValidSignature},
	{Label: "Echo", URL: "/c/ig/receive", Data: echo, Status: 200, Response: `ignoring echo`, PrepRequest: addValidSignature},
	{Label: "Not Instagram", URL: "/c/ig/receive", Data: notInstagram, Status: 400, Response: "expected 'instagram', found notinstagram", PrepRequest: addValidSignature},
	{Label: "No Entries", URL: "/c/ig/receive", Data: noEntries, Status: 400, Response: "no entries found", PrepRequest: addValidSignature},
	{Label: "No Messaging Entries", URL: "/c/ig/receive", Data: noMessagingEntries, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Unknown Messaging Entry", URL: "/c/ig/receive", Data: unkownMessagingEntry, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Not JSON", URL: "/c/ig/receive", Data: notJSON, Status: 400, Response: "Error", PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: "/c/ig/receive", Data: invalidURN, Status: 400, Response: "invalid instagram id", PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, 100000)
	sig, _ := fbCalculateSignature("ig_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha1=%s", string(sig)))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

// mocks the call to the Facebook graph API
func buildMockFBGraph(testCases []ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", 403)
		}

		// user has a name
		if strings.HasSuffix(r.URL.Path, "1337") {
			w.Write([]byte(`{ "first_name": "John", "last_name": "Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "first_name": "", "last_name": ""}`))
	}))
	graphURL = server.URL

	return server
}

func TestDescribe(t *testing.T) {
	fbGraph := buildMockFBGraph(testCases)
	defer fbGraph.Close()

	handler := newHandler().(courier.URNDescriber)
	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{{"instagram:1337", map[string]string{"name": "John Doe"}},
		{"instagram:4567", map[string]string{"name": ""}}}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	fbService := buildMockFBGraph(testCases)
	defer fbService.Close()

	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func TestVerify(t *testing.T) {

	RunChannelTestCases(t, testChannels, newHandler(), []ChannelHandleTestCase{
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=ig_webhook_secret&hub.challenge=yarchallenge", Status: 200,
			Response: "yarchallenge", NoQueueErrorCheck: true, NoInvalidChannelCheck: true},
		{Label: "Verify No Mode", URL: "/c/ig/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/ig/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=ig_webhook_secret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},

	{Label: "Plain Response",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133", ResponseToID: 23526,
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},

	{Label: "Tag Human Agent",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133", Topic: "agent",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},

	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "instagram:12345", QuickReplies: []string{"Yes", "No"}, Topic: "agent",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},

	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "instagram:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"},
		Status:       "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},

	{Label: "ID Error",
		Text: "ID Error", URN: "instagram12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 200,
		SendPrep: setSendURL},

	{Label: "Error",
		Text: "Error", URN: "instagram12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 403,
		SendPrep: setSendURL},

	{Label: "Quick Reply",
		URN: "instagram:12345", Text: "Are you happy?", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},

	{Label: "Send Photo",
		URN: "instagram:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "2020", "US", map[string]interface{}{courier.ConfigAuthToken: "access_token"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"308de7627fe19e92294c4572a7f831bc1002809d",
		},
		{
			"hello world2",
			"ab6f902b58b9944032d4a960f470d7a8ebfd12b7",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}
