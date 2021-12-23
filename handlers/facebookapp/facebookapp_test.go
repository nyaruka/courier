package facebookapp

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
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testChannelsIG = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var helloMsgFBA = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var helloMsgIG = `{
	"object":"instagram",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var duplicateMsgFBA = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	},
	{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var duplicateMsgIG = `{
	"object":"instagram",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	},
	{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var invalidURNFBA = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "abc5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var invalidURNIG = `{
	"object":"instagram",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"message": {
			  "text": "Hello World",
			  "mid": "external_id"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "abc5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var attachmentFBA = `{
	"object":"page",
	"entry": [{
	  	"id": "12345",
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
					"id": "12345"
				},
				"sender": {
					"id": "5678"
				},
				"timestamp": 1459991487970
	    }],
	  	"time": 1459991487970
	}]
}`

var attachmentIG = `{
	"object":"instagram",
	"entry": [{
	  	"id": "12345",
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
					"id": "12345"
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
	  	"id": "12345",
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
					"id": "12345"
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
		"id":"12345",
		"time":1459991487970,
		"messaging":[{
			"sender":{"id":"5678"},
			"recipient":{"id":"12345"},
			"timestamp":1459991487970,
			"message":{
				"mid":"external_id",
				"attachments":[{
					"type":"image",
					"payload":{
						"sticker_id": 369239263222822,
						"url":"https://scontent.xx.fbcdn.net/v/arst"
					}
				}]
			}
		}]
	}]
}`

var like_heart = `{
	"object":"instagram",
	"entry":[{
		"id":"12345",
		"messaging":[{
			"sender":{"id":"5678"},
			"recipient":{"id":"12345"},
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

var differentPageIG = `{
	"object":"instagram",
	"entry": [{
	  "id": "12345",
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

var differentPageFBA = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
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

var echoFBA = `{
	"object":"page",
	"entry": [{
		"id": "12345",
		"messaging": [{
			"recipient": {
				"id": "12345"
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

var echoIG = `{
	"object":"instagram",
	"entry": [{
		"id": "12345",
		"messaging": [{
			"recipient": {
				"id": "12345"
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
	  "id": "12345",
	  "messaging": [{
			"postback": {
				"title": "icebreaker question",  
				"payload": "get_started"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var optInUserRef = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
		  "optin": {
		  	"ref": "optin_ref",
		  	"user_ref": "optin_user_ref"
		  },
		  "recipient": {
		  	"id": "12345"
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
	  "id": "12345",
	  "messaging": [{
			"optin": {
		 		"ref": "optin_ref"
			},
			"recipient": {
		  	"id": "12345"
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
	  "id": "12345",
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
			  "id": "12345"
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
	  "id": "12345",
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
			  "id": "12345"
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
	  "id": "12345",
	  "messaging": [{
			"postback": {
				"title": "postback title",  
				"payload": "get_started"
			},
			"recipient": {
			  "id": "12345"
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
	  "id": "12345",
	  "messaging": [{
			"referral": {
				"ref": "referral id",
				"ad_id": "ad id",
				"source": "referral source",
				"type": "referral type"
			},
			"recipient": {
			  "id": "12345"
			},
			"sender": {
			  "id": "5678",
			  "user_ref": "5678"
			},
			"timestamp": 1459991487970
	  }],
	  "time": 1459991487970
	}]
}`

var dlr = `{
	"object":"page",
	"entry": [{
	  "id": "12345",
	  "messaging": [{
			"delivery":{
				"mids":[
				   "mid.1458668856218:ed81099e15d3f4f233"
				],
				"watermark":1458668856253,
				"seq":37
			},
			"recipient": {
			  "id": "12345"
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

var notInstagram = `{
	"object":"notinstagram",
	"entry": [{}]
}`

var noEntriesFBA = `{
	"object":"page",
	"entry": []
}`

var noEntriesIG = `{
	"object":"instagram",
	"entry": []
}`

var noMessagingEntriesFBA = `{
	"object":"page",
	"entry": [{
		"id": "12345"
	}]
}`

var noMessagingEntriesIG = `{
	"object":"instagram",
	"entry": [{
		"id": "12345"
	}]
}`

var unkownMessagingEntryFBA = `{
	"object":"page",
	"entry": [{
		"id": "12345",
		"messaging": [{
			"recipient": {
				"id": "12345"
			},
			"sender": {
				"id": "5678"
			},
			"timestamp": 1459991487970
		}]
	}]
}`

var unkownMessagingEntryIG = `{
	"object":"instagram",
	"entry": [{
		"id": "12345",
		"messaging": [{
			"recipient": {
				"id": "12345"
			},
			"sender": {
				"id": "5678"
			},
			"timestamp": 1459991487970
		}]
	}]
}`

var notJSON = `blargh`

var testCasesFBA = []ChannelHandleTestCase{
	{Label: "Receive Message FBA", URL: "/c/fba/receive", Data: helloMsgFBA, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: "/c/fba/receive", Data: helloMsgFBA, Status: 400, Response: "invalid request signature", PrepRequest: addInvalidSignature},

	{Label: "No Duplicate Receive Message", URL: "/c/fba/receive", Data: duplicateMsgFBA, Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Attachment", URL: "/c/fba/receive", Data: attachmentFBA, Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Location", URL: "/c/fba/receive", Data: locationAttachment, Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"geo:1.200000,-1.300000"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Thumbs Up", URL: "/c/fba/receive", Data: thumbsUp, Status: 200, Response: "Handled",
		Text: Sp("👍"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive OptIn UserRef", URL: "/c/fba/receive", Data: optInUserRef, Status: 200, Response: "Handled",
		URN: Sp("facebook:ref:optin_user_ref"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest: addValidSignature},
	{Label: "Receive OptIn", URL: "/c/fba/receive", Data: optIn, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest: addValidSignature},

	{Label: "Receive Get Started", URL: "/c/fba/receive", Data: postbackGetStarted, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started"},
		PrepRequest:       addValidSignature},
	{Label: "Receive Referral Postback", URL: "/c/fba/receive", Data: postback, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"},
		PrepRequest:       addValidSignature},
	{Label: "Receive Referral", URL: "/c/fba/receive", Data: postbackReferral, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type", "ad_id": "ad id"},
		PrepRequest:       addValidSignature},

	{Label: "Receive Referral", URL: "/c/fba/receive", Data: referral, Status: 200, Response: `"referrer_id":"referral id"`,
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"},
		PrepRequest:       addValidSignature},

	{Label: "Receive DLR", URL: "/c/fba/receive", Data: dlr, Status: 200, Response: "Handled",
		Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), MsgStatus: Sp(courier.MsgDelivered), ExternalID: Sp("mid.1458668856218:ed81099e15d3f4f233"),
		PrepRequest: addValidSignature},

	{Label: "Different Page", URL: "/c/fba/receive", Data: differentPageFBA, Status: 200, Response: `"data":[]`, PrepRequest: addValidSignature},
	{Label: "Echo", URL: "/c/fba/receive", Data: echoFBA, Status: 200, Response: `ignoring echo`, PrepRequest: addValidSignature},
	{Label: "Not Page", URL: "/c/fba/receive", Data: notPage, Status: 400, Response: "object expected 'page' or 'instagram', found notpage", PrepRequest: addValidSignature},
	{Label: "No Entries", URL: "/c/fba/receive", Data: noEntriesFBA, Status: 400, Response: "no entries found", PrepRequest: addValidSignature},
	{Label: "No Messaging Entries", URL: "/c/fba/receive", Data: noMessagingEntriesFBA, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Unknown Messaging Entry", URL: "/c/fba/receive", Data: unkownMessagingEntryFBA, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Not JSON", URL: "/c/fba/receive", Data: notJSON, Status: 400, Response: "Error", PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: "/c/fba/receive", Data: invalidURNFBA, Status: 400, Response: "invalid facebook id", PrepRequest: addValidSignature},
}
var testCasesIG = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: "/c/ig/receive", Data: helloMsgIG, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Invalid Signature", URL: "/c/ig/receive", Data: helloMsgIG, Status: 400, Response: "invalid request signature", PrepRequest: addInvalidSignature},

	{Label: "No Duplicate Receive Message", URL: "/c/ig/receive", Data: duplicateMsgIG, Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Attachment", URL: "/c/ig/receive", Data: attachmentIG, Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Like Heart", URL: "/c/ig/receive", Data: like_heart, Status: 200, Response: "Handled",
		Text: Sp(""), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Icebreaker Get Started", URL: "/c/ig/receive", Data: icebreakerGetStarted, Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "icebreaker question", "payload": "get_started"},
		PrepRequest:       addValidSignature},

	{Label: "Different Page", URL: "/c/ig/receive", Data: differentPageIG, Status: 200, Response: `"data":[]`, PrepRequest: addValidSignature},
	{Label: "Echo", URL: "/c/ig/receive", Data: echoIG, Status: 200, Response: `ignoring echo`, PrepRequest: addValidSignature},
	{Label: "No Entries", URL: "/c/ig/receive", Data: noEntriesIG, Status: 400, Response: "no entries found", PrepRequest: addValidSignature},
	{Label: "Not Instagram", URL: "/c/ig/receive", Data: notInstagram, Status: 400, Response: "object expected 'page' or 'instagram', found notinstagram", PrepRequest: addValidSignature},
	{Label: "No Messaging Entries", URL: "/c/ig/receive", Data: noMessagingEntriesIG, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Unknown Messaging Entry", URL: "/c/ig/receive", Data: unkownMessagingEntryIG, Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Not JSON", URL: "/c/ig/receive", Data: notJSON, Status: 400, Response: "Error", PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: "/c/ig/receive", Data: invalidURNIG, Status: 400, Response: "invalid facebook id", PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, 100000)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
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
			w.Write([]byte(`{ "name": "John Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "name": ""}`))
	}))
	graphURL = server.URL

	return server
}

func TestDescribe(t *testing.T) {
	var testCases [][]ChannelHandleTestCase
	testCases = append(testCases, testCasesFBA)
	testCases = append(testCases, testCasesIG)

	for i, tc := range testCases {
		fbGraph := buildMockFBGraph(tc)
		defer fbGraph.Close()

		if i == 0 {
			handler := newHandler("FBA", "Facebook", false).(courier.URNDescriber)
			tcs := []struct {
				urn      urns.URN
				metadata map[string]string
			}{
				{"facebook:1337", map[string]string{"name": "John Doe"}},
				{"facebook:4567", map[string]string{"name": ""}},
			}

			for _, tc := range tcs {
				metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn)
				assert.Equal(t, metadata, tc.metadata)
			}
		} else {
			handler := newHandler("IG", "Instagram", false).(courier.URNDescriber)
			tcs := []struct {
				urn      urns.URN
				metadata map[string]string
			}{
				{"facebook:1337", map[string]string{"name": "John Doe"}},
				{"facebook:4567", map[string]string{"name": ""}},
			}

			for _, tc := range tcs {
				metadata, _ := handler.DescribeURN(context.Background(), testChannelsIG[0], tc.urn)
				assert.Equal(t, metadata, tc.metadata)
			}
		}

	}

}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler("FBA", "Facebook", false), testCasesFBA)
	RunChannelTestCases(t, testChannelsIG, newHandler("IG", "Instagram", false), testCasesIG)

}

func BenchmarkHandler(b *testing.B) {
	fbService := buildMockFBGraph(testCasesFBA)

	RunChannelBenchmarks(b, testChannels, newHandler("FBA", "Facebook", false), testCasesFBA)
	fbService.Close()

	fbServiceIG := buildMockFBGraph(testCasesIG)

	RunChannelBenchmarks(b, testChannelsIG, newHandler("IG", "Instagram", false), testCasesIG)
	fbServiceIG.Close()
}

func TestVerify(t *testing.T) {

	RunChannelTestCases(t, testChannels, newHandler("FBA", "Facebook", false), []ChannelHandleTestCase{
		{Label: "Valid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200,
			Response: "yarchallenge", NoQueueErrorCheck: true, NoInvalidChannelCheck: true},
		{Label: "Verify No Mode", URL: "/c/fba/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/fba/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

	RunChannelTestCases(t, testChannelsIG, newHandler("IG", "Instagram", false), []ChannelHandleTestCase{
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200,
			Response: "yarchallenge", NoQueueErrorCheck: true, NoInvalidChannelCheck: true},
		{Label: "Verify No Mode", URL: "/c/ig/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/ig/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var SendTestCasesFBA = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Response",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133", ResponseToExternalID: "23526",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Send using ref URN",
		Text: "Simple Message", URN: "facebook:ref:67890",
		ContactURNs: map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		Status:      "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133", "recipient_id": "12345"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "facebook:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "facebook:12345", QuickReplies: []string{"Yes", "No"}, Topic: "account",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"ACCOUNT_UPDATE","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Photo",
		URN: "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"}, Topic: "event",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"CONFIRMED_EVENT_UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Document",
		URN: "facebook:12345", Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
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

var SendTestCasesIG = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Response",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133", ResponseToExternalID: "23526",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "facebook:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "facebook:12345", QuickReplies: []string{"Yes", "No"}, Topic: "agent",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Photo",
		URN: "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"},
		Status:       "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Tag Human Agent",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133", Topic: "agent",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Send Document",
		URN: "facebook:12345", Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
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
	var ChannelFBA = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	var ChannelIG = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	RunChannelSendTestCases(t, ChannelFBA, newHandler("FBA", "Facebook", false), SendTestCasesFBA, nil)
	RunChannelSendTestCases(t, ChannelIG, newHandler("IG", "Instagram", false), SendTestCasesIG, nil)
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
