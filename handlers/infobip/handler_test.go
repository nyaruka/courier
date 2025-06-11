package infobip

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
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
	receiveURL = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
)

var helloMsg = `{
  	"results": [
		{
			"messageId": "817790313235066447",
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

var invalidURN = `{
	"results": [
		{
			"messageId": "817790313235066447",
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
		  "messageId": "817790313235066447",
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
			"messageId": "817790313235066447",
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
			"messageId": "12345",
			"status": {
				"groupName": "DELIVERED"
			}
		}
	]
}`

var validStatusDelivered = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "DELIVERED"
			}
		}
	]
}`

var validStatusRejected = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "REJECTED"
			}
		}
	]
}`

var validStatusUndeliverable = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "UNDELIVERABLE"
			}
		}
	]
}`

var validStatusPending = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "PENDING"
			}
		},
		{
			"messageId": "12347",
			"status": {
				"groupName": "PENDING"
			}
		}		
	]
}`

var validStatusExpired = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "EXPIRED"
			}
		}
	]
}`

var invalidStatus = `{
	"results": [
		{
			"messageId": "12345",
			"status": {
				"groupName": "UNEXPECTED"
			}
		}
	]
}`

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("QUIZ Correct answer is Paris"),
		ExpectedURN:          "tel:+385916242493",
		ExpectedExternalID:   "817790313235066447",
		ExpectedDate:         time.Date(2016, 10, 06, 9, 28, 39, 220000000, time.FixedZone("", 0)),
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
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Status rejected",
		URL:                  statusURL,
		Data:                 validStatusRejected,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Status undeliverable",
		URL:                  statusURL,
		Data:                 validStatusUndeliverable,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Status pending",
		URL:                  statusURL,
		Data:                 validStatusPending,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusSent}, {ExternalID: "12347", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Status expired",
		URL:                  statusURL,
		Data:                 validStatusExpired,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusSent}},
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

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "12345"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		}},
		ExpectedExtIDs: []string{"12345"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"☺","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("messageId")},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"My pic!\nhttps://foo.bar/image.jpg","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("messageId")},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Error Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error groupId",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 2}}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
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
			"https://api.infobip.com/sms/1/text/advanced": {
				httpx.NewMockResponse(200, nil, []byte(`{"messages":[{"status":{"groupId": 1}, "messageId": "12345"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
			},
			Body: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","transliteration":"COLOMBIAN"}]}`,
		}},
		ExpectedExtIDs: []string{"12345"},
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)

	var transChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
			configTransliteration:  "COLOMBIAN",
		})

	RunOutgoingTestCases(t, transChannel, newHandler(), transSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)
}
