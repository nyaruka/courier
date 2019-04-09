package facebook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FB", "1234", "",
		map[string]interface{}{courier.ConfigAuthToken: "a123", courier.ConfigSecret: "mysecret"}),
}

var helloMsg = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
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
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
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
	  "id": "208685479508187",
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
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
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
	"object":"page",
	"entry": [{
	  	"id": "208685479508187",
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

var differentPage = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
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
	"object":"page",
	"entry": [{
		"id": "208685479508187",
		"messaging": [{
			"recipient": {
				"id": "1234"
			},
			"sender": {
				"id": "5678"
			},
			"timestamp": 1459991487970,
			"message": {
				"is_echo": true
			}
		}]
	}]
}`

var optInUserRef = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"optin": {
		  	"ref": "optin_ref",
		  	"user_ref": "optin_user_ref"
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

var optIn = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"optin": {
		 		"ref": "optin_ref"
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

var postback = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"postback": {
				"title": "postback title",  
				"payload": "postback payload",
				"referral": {
				  "ref": "postback ref",
				  "source": "postback source",
				  "type": "postback type"
				}
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

var postbackReferral = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"postback": {
				"title": "postback title",  
				"payload": "get_started",
				"referral": {
				  "ref": "postback ref",
				  "source": "postback source",
				  "type": "postback type"
				}
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

var postbackGetStarted = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"postback": {
				"title": "postback title",  
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

var referral = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"referral": {
				"ref": "referral id",
				"ad_id": "ad id",
				"source": "referral source",
				"type": "referral type"
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

var dlr = `{
	"object":"page",
	"entry": [{
	  "id": "208685479508187",
	  "messaging": [{
			"delivery":{
				"mids":[
				   "mid.1458668856218:ed81099e15d3f4f233"
				],
				"watermark":1458668856253,
				"seq":37
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

var notPage = `{
	"object":"notpage",
	"entry": [{}]
}`

var noEntries = `{
	"object":"page",
	"entry": []
}`

var noMessagingEntries = `{
	"object":"page",
	"entry": [{}]
}`

var unkownMessagingEntry = `{
	"object":"page",
	"entry": [{
		"id": "208685479508187",
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
	{Label: "Receive Message", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},
	{Label: "No Duplicate Receive Message", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: duplicateMsg, Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},
	{Label: "Receive Attachment", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: attachment, Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},

	{Label: "Receive OptIn UserRef", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: optInUserRef, Status: 200, Response: "Handled",
		URN: Sp("facebook:ref:optin_user_ref"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"}},
	{Label: "Receive OptIn", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: optIn, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"}},

	{Label: "Receive Get Started", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: postbackGetStarted, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started"}},
	{Label: "Receive Referral Postback", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: postback, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"}},
	{Label: "Receive Referral", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: postbackReferral, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"}},

	{Label: "Receive Referral", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: referral, Status: 200, Response: `"referrer_id":"referral id"`,
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"}},

	{Label: "Receive DLR", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: dlr, Status: 200, Response: "Handled",
		Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), MsgStatus: Sp(courier.MsgDelivered), ExternalID: Sp("mid.1458668856218:ed81099e15d3f4f233")},

	{Label: "Different Page", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: differentPage, Status: 200, Response: `"data":[]`},
	{Label: "Echo", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: echo, Status: 200, Response: `ignoring echo`},
	{Label: "Not Page", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: notPage, Status: 200, Response: "ignoring"},
	{Label: "No Entries", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: noEntries, Status: 200, Response: "ignoring"},
	{Label: "No Messaging Entries", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: noMessagingEntries, Status: 200, Response: "Handled"},
	{Label: "Unknown Messaging Entry", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: unkownMessagingEntry, Status: 200, Response: "Handled"},
	{Label: "Not JSON", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: notJSON, Status: 400, Response: "Error"},
	{Label: "Invalid URN", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidURN, Status: 400, Response: "invalid facebook id"},
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
	}{{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
		{"facebook:ref:1337", map[string]string{}}}

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
	subscribeCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.FormValue("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			fmt.Printf("Access token: %s\n", accessToken)
			http.Error(w, "invalid auth token", 403)
			return
		}

		// valid token
		w.Write([]byte(`{"success": true}`))

		// mark that we were called
		subscribeCalled = true
	}))
	subscribeURL = server.URL
	subscribeTimeout = time.Millisecond

	RunChannelTestCases(t, testChannels, newHandler(), []ChannelHandleTestCase{
		{Label: "Receive Message", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200},
		{Label: "Verify No Mode", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe&hub.verify_token=mysecret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

	// wait for our subscribe to be called
	time.Sleep(100 * time.Millisecond)

	if !subscribeCalled {
		t.Error("subscribe endpoint should have been called")
	}
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Send using ref URN",
		Text: "Simple Message", URN: "facebook:ref:67890",
		ContactURNs: map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		Status:      "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133", "recipient_id": "12345"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "facebook:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "facebook:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Photo",
		URN: "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"},
		Status:       "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "ID Error",
		Text: "ID Error", URN: "facebook:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error",
		Text: "Error", URN: "facebook:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FB", "2020", "US", map[string]interface{}{courier.ConfigAuthToken: "access_token"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
