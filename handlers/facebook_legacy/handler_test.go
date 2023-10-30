package facebook_legacy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FB", "1234", "",
		map[string]any{courier.ConfigAuthToken: "a123", courier.ConfigSecret: "mysecret"}),
}

const (
	receiveURL = "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
)

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

var locationAttachment = `{
	"object":"page",
	"entry": [{
	  	"id": "208685479508187",
	  	"messaging": [{
				"message": {
		  			"mid": "external_id",
		  			"attachments":[{
						"type":"location",
      	      			"payload":{
							"coordinates": {
								"lat": 1.2,
								"long": -1.3
							}
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

var thumbsUp = `{
	"object":"page",
	"entry":[{
		"id":"208685479508187",
		"time":1459991487970,
		"messaging":[{
			"sender":{"id":"5678"},
			"recipient":{"id":"1234"},
			"timestamp":1459991487970,
			"message":{
				"mid":"external_id",
				"attachments":[{
					"type":"image",
					"payload":{
						"url":"https://scontent.xx.fbcdn.net/v/arst",
						"sticker_id":369239263222822
					}
				}],
				"sticker_id":369239263222822
			}
		}]
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
				  "type": "postback type",
				  "ad_id": "ad id"
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

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Message",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "No Duplicate Receive Message",
		URL:                  receiveURL,
		Data:                 duplicateMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "Receive Attachment",
		URL:                  receiveURL,
		Data:                 attachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://image-url/foo.png"},
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "Receive Location",
		URL:                  receiveURL,
		Data:                 locationAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:1.200000,-1.300000"},
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "Receive Thumbs Up",
		URL:                  receiveURL,
		Data:                 thumbsUp,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("👍"),
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "Receive OptIn UserRef",
		URL:                  receiveURL,
		Data:                 optInUserRef,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:ref:optin_user_ref", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "optin_ref"}},
		},
	},
	{
		Label:                "Receive OptIn",
		URL:                  receiveURL,
		Data:                 optIn,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "optin_ref"}},
		},
	},
	{
		Label:                "Receive Get Started",
		URL:                  receiveURL,
		Data:                 postbackGetStarted,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "get_started"}},
		},
	},
	{
		Label:                "Receive Referral Postback",
		URL:                  receiveURL,
		Data:                 postback,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"}},
		},
	},
	{
		Label:                "Receive Referral",
		URL:                  receiveURL,
		Data:                 postbackReferral,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type", "ad_id": "ad id"}},
		},
	},
	{
		Label:                "Receive Referral",
		URL:                  receiveURL,
		Data:                 referral,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"referrer_id":"referral id"`,
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"}},
		},
	},
	{
		Label:                "Receive DLR",
		URL:                  receiveURL,
		Data:                 dlr,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "mid.1458668856218:ed81099e15d3f4f233", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Different Page",
		URL:                  receiveURL,
		Data:                 differentPage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"data":[]`,
	},
	{
		Label:                "Echo",
		URL:                  receiveURL,
		Data:                 echo,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring echo`,
	},
	{
		Label:                "Not Page",
		URL:                  receiveURL,
		Data:                 notPage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring",
	},
	{
		Label:                "No Entries",
		URL:                  receiveURL,
		Data:                 noEntries,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring",
	},
	{
		Label:                "No Messaging Entries",
		URL:                  receiveURL,
		Data:                 noMessagingEntries,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
	},
	{
		Label:                "Unknown Messaging Entry",
		URL:                  receiveURL,
		Data:                 unkownMessagingEntry,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
	},
	{
		Label:                "Not JSON",
		URL:                  receiveURL,
		Data:                 `blargh`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid facebook id",
	},
}

// mocks the call to the Facebook graph API
func buildMockFBGraph(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", http.StatusForbidden)
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

func TestDescribeURN(t *testing.T) {
	fbGraph := buildMockFBGraph(testCases)
	defer fbGraph.Close()

	channel := testChannels[0]
	handler := newHandler()
	handler.Initialize(test.NewMockServer(courier.NewConfig(), test.NewMockBackend()))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
		{"facebook:ref:1337", map[string]string{}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(courier.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "mysecret"})
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
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
			http.Error(w, "invalid auth token", http.StatusForbidden)
			return
		}

		// valid token
		w.Write([]byte(`{"success": true}`))

		// mark that we were called
		subscribeCalled = true
	}))
	subscribeURL = server.URL
	subscribeTimeout = time.Millisecond

	RunIncomingTestCases(t, testChannels, newHandler(), []IncomingTestCase{
		{
			Label:              "Receive Message",
			URL:                receiveURL,
			Data:               helloMsg,
			ExpectedRespStatus: 200,
			ExpectedMsgText:    Sp("Hello World"),
			ExpectedURN:        "facebook:5678",
		},
		{
			Label:                "Verify No Mode",
			URL:                  receiveURL,
			ExpectedRespStatus:   400,
			ExpectedBodyContains: "unknown request",
		},
		{
			Label:                "Verify No Secret",
			URL:                  "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe",
			ExpectedRespStatus:   400,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Invalid Secret",
			URL:                  "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe&hub.verify_token=blah",
			ExpectedRespStatus:   400,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Valid Secret",
			URL:                  "/c/fb/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?hub.mode=subscribe&hub.verify_token=mysecret&hub.challenge=yarchallenge",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "yarchallenge",
		},
	})

	// wait for our subscribe to be called
	time.Sleep(100 * time.Millisecond)

	if !subscribeCalled {
		t.Error("subscribe endpoint should have been called")
	}
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:12345",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Plain Response",
		MsgText:                 "Simple Message",
		MsgURN:                  "facebook:12345",
		MsgResponseToExternalID: "23526",
		MockResponseBody:        `{"message_id": "mid.133"}`,
		MockResponseStatus:      200,
		ExpectedRequestBody:     `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:       "W",
		ExpectedExternalID:      "mid.133",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Plain Send using ref URN",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:ref:67890",
		MockResponseBody:    `{"message_id": "mid.133", "recipient_id": "12345"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		ExpectedContactURNs: map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "facebook:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Message",
		MsgText:             "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		MsgURN:              "facebook:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MsgTopic:            "account",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"ACCOUNT_UPDATE","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Photo",
		MsgURN:              "facebook:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send caption and photo with Quick Reply",
		MsgText:             "This is some text.",
		MsgURN:              "facebook:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgQuickReplies:     []string{"Yes", "No"},
		MsgTopic:            "event",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"CONFIRMED_EVENT_UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Document",
		MsgURN:              "facebook:12345",
		MsgAttachments:      []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"NON_PROMOTIONAL_SUBSCRIPTION","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:              "ID Error",
		MsgText:            "ID Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FB", "2020", "US", map[string]any{courier.ConfigAuthToken: "access_token"})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"access_token"}, nil)
}
