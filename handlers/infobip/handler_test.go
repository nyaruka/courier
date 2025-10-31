package infobip

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US", []string{urns.Phone.Prefix}, nil),
}

const (
	receiveURL = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL  = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
)

var helloMsg = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"from": "385916242493",
			"to": "385921004026",
			"receivedAt": "2016-10-06T09:28:39.220+0000",
			"message": [
				{
					"contentType": "text/plain",
					"value": "This is message text"
				},
				{
					"contentType": "image/jpeg",
					"url": "https://examplelink.com/123456"
				}
			],
			"price": {
				"pricePerMessage": 0,
				"currency": "EUR"
			}
		}
	],
	"messageCount": 1,
	"pendingMessageCount": 0
}`

var invalidURN = `{
	"results": [
		{
		 "messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"from": "MTN",
			"to": "385921004026",
			"text": "QUIZ Correct answer is Paris",
			"cleanText": "Correct answer is Paris",
			"keyword": "QUIZ",
			"receivedAt": "2016-10-06T09:28:39.220+0000",
			"smsCount": 1,
			"price": {
				"pricePerMessage": 0,
				"currency": "EUR"
			},
			"callbackData": "callbackData"
		}
	],
	"messageCount": 1,
	"pendingMessageCount": 0
}`

var missingResults = `{
	"unexpected": [
	  {
	  	"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
		  "from": "385916242493",
		  "to": "385921004026",
		  "text": "QUIZ Correct answer is Paris",
		  "cleanText": "Correct answer is Paris",
		  "keyword": "QUIZ",
		  "receivedAt": "2016-10-06T09:28:39.220+0000",
		  "smsCount": 1,
		  "price": {
		   "pricePerMessage": 0,
		   "currency": "EUR"
		  },
		  "callbackData": "callbackData"
	  }
  ],
  "messageCount": 1,
  "pendingMessageCount": 0
}`

var missingText = `{
  	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"from": "385916242493",
			"to": "385921004026",
			"text": "",
			"cleanText": "Correct answer is Paris",
			"keyword": "QUIZ",
			"receivedAt": "2016-10-06T09:28:39.220+0000",
			"smsCount": 1,
			"price": {
				"pricePerMessage": 0,
				"currency": "EUR"
			},
			"callbackData": "callbackData"
		}
	],
	"messageCount": 1,
	"pendingMessageCount": 0
}`

var invalidJSONStatus = "Invalid"

var statusMissingResultsKey = `{
	"deliveryReport": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "DELIVERED"
			}
		}
	]
}`

var validStatusDelivered = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "DELIVERED"
			}
		}
	]
}`

var validStatusRejected = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "REJECTED"
			}
		}
	]
}`

var validStatusUndeliverable = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "UNDELIVERABLE"
			}
		}
	]
}`

var validStatusPending = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "PENDING"
			}
		},
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "PENDING"
			}
		}		
	]
}`

var validStatusExpired = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "EXPIRED"
			}
		}
	]
}`

var invalidStatus = `{
	"results": [
		{
			"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
			"status": {
				"groupName": "UNEXPECTED"
			}
		}
	]
}`

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid MMS Message",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("This is message text"),
		ExpectedURN:          "tel:+385916242493",
		ExpectedExternalID:   "0191e180-7d60-7000-aded-7d8b151cbd5b",
		ExpectedDate:         time.Date(2016, 10, 06, 9, 28, 39, 220000000, time.UTC),
		ExpectedAttachments:  []string{"image/jpeg:https://examplelink.com/123456"},
	},
	{
		Label: "Receive Valid SMS Message",
		URL:   receiveURL,
		Data: `{
			"results": [
				{
					"messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b",
					"from": "385916242494",
					"to": "385921004027",
					"text": "This is an SMS message",
					"receivedAt": "2016-10-06T09:28:40.000+0000",
					"smsCount": 1,
					"price": {
						"pricePerMessage": 0,
						"currency": "EUR"
					}
				}
			],
			"messageCount": 1,
			"pendingMessageCount": 0
		}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("This is an SMS message"),
		ExpectedURN:          "tel:+385916242494",
		ExpectedExternalID:   "0191e180-7d60-7000-aded-7d8b151cbd5b",
		ExpectedDate:         time.Date(2016, 10, 06, 9, 28, 40, 0, time.UTC),
		ExpectedAttachments:  []string{},
	},
	{
		Label:                "Receive missing results key",
		URL:                  receiveURL,
		Data:                 missingResults,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'Results' failed",
	},
	{
		Label:                "Receive missing text key",
		URL:                  receiveURL,
		Data:                 missingText,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring request, no message",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Status report invalid JSON",
		URL:                  statusURL,
		Data:                 invalidJSONStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request JSON",
	},
	{
		Label:                "Status report missing results key",
		URL:                  statusURL,
		Data:                 statusMissingResultsKey,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'Results' failed",
	},
	{
		Label:                "Status delivered",
		URL:                  statusURL,
		Data:                 validStatusDelivered,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusDelivered}},
	},
	{
		Label:                "Status rejected",
		URL:                  statusURL,
		Data:                 validStatusRejected,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusFailed}},
	},
	{
		Label:                "Status undeliverable",
		URL:                  statusURL,
		Data:                 validStatusUndeliverable,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusFailed}},
	},
	{
		Label:                "Status pending",
		URL:                  statusURL,
		Data:                 validStatusPending,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusSent}, {ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusSent}},
	},
	{
		Label:                "Status expired",
		URL:                  statusURL,
		Data:                 validStatusExpired,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "0191e180-7d60-7000-aded-7d8b151cbd5b", Status: models.MsgStatusSent}},
	},
	{
		Label:                "Status group name unexpected",
		URL:                  statusURL,
		Data:                 invalidStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unknown status 'UNEXPECTED'`,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"Simple Message"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedExtIDs: []string{"0191e180-7d60-7000-aded-7d8b151cbd5b"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"☺"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("messageId")},
	},
	{
		Label:          "Send MMS with Attachment",
		MsgText:        "Check out this image!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://example.com/my_image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/mms/2/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"sender":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"title":"","messageSegments":[{"type":"TEXT","text":"Check out this image!"},{"type":"IMAGE","url":"https://example.com/my_image.jpg"}]},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedExtIDs: []string{"0191e180-7d60-7000-aded-7d8b151cbd5b"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"Error Message"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error groupId",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 2}}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"Simple Message"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedError: courier.ErrResponseContent,
	},
}

var transSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"Simple Message","transliteration":"COLOMBIAN"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedExtIDs: []string{"0191e180-7d60-7000-aded-7d8b151cbd5b"},
	},
}

var apiKeySendTestCases = []OutgoingTestCase{
	{
		Label:   "API Key Send",
		MsgText: "API Key Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/3/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "0191e180-7d60-7000-aded-7d8b151cbd5b"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "App test-api-key",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"0191e180-7d60-7000-aded-7d8b151cbd5b"}],"content":{"text":"API Key Message"},"webhooks":{"delivery":{"url":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","intermediateReport":true,"contentType":"application/json"}}}]}`,
		}},
		ExpectedExtIDs: []string{"0191e180-7d60-7000-aded-7d8b151cbd5b"},
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)

	var transChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
			configTransliteration: "COLOMBIAN",
		})

	RunOutgoingTestCases(t, transChannel, newHandler(), transSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)

	var apiKeyChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigAPIKey: "test-api-key",
		})

	RunOutgoingTestCases(t, apiKeyChannel, newHandler(), apiKeySendTestCases, []string{"App test-api-key"}, nil)
}
