package infobip

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US", nil),
}

var receiveURL = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
var statusURL = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: helloMsg, Status: 200, Response: "Accepted",
		Text: Sp("QUIZ Correct answer is Paris"), URN: Sp("tel:+385916242493"), ExternalID: Sp("817790313235066447"), Date: Tp(time.Date(2016, 10, 06, 9, 28, 39, 220000000, time.FixedZone("", 0)))},
	{Label: "Receive missing results key", URL: receiveURL, Data: missingResults, Status: 400, Response: "validation for 'Results' failed"},
	{Label: "Receive missing text key", URL: receiveURL, Data: missingText, Status: 200, Response: "ignoring request, no message"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Status report invalid JSON", URL: statusURL, Data: invalidJSONStatus, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Status report missing results key", URL: statusURL, Data: statusMissingResultsKey, Status: 400, Response: "Field validation for 'Results' failed"},
	{Label: "Status delivered", URL: statusURL, Data: validStatusDelivered, Status: 200, Response: `"status":"D"`},
	{Label: "Status rejected", URL: statusURL, Data: validStatusRejected, Status: 200, Response: `"status":"F"`},
	{Label: "Status undeliverable", URL: statusURL, Data: validStatusUndeliverable, Status: 200, Response: `"status":"F"`},
	{Label: "Status pending", URL: statusURL, Data: validStatusPending, Status: 200, Response: `"status":"S"`},
	{Label: "Status expired", URL: statusURL, Data: validStatusExpired, Status: 200, Response: `"status":"S"`},
	{Label: "Status group name unexpected", URL: statusURL, Data: invalidStatus, Status: 400, Response: `unknown status 'UNEXPECTED'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "12345",
		ResponseBody: `{"messages":[{"status":{"groupId": 1}, "messageId": "12345"}}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `{"messages":[{"status":{"groupId": 1}}}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"☺","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{"messages":[{"status":{"groupId": 1}}}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"My pic!\nhttps://foo.bar/image.jpg","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Error Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		SendPrep:    setSendURL},
	{Label: "Error groupId",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{"messages":[{"status":{"groupId": 2}}}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered"}]}`,
		SendPrep:    setSendURL},
}

var transSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "12345",
		ResponseBody: `{"messages":[{"status":{"groupId": 1}, "messageId": "12345"}}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic VXNlcm5hbWU6UGFzc3dvcmQ=",
		},
		RequestBody: `{"messages":[{"from":"2020","destinations":[{"to":"250788383383","messageId":"10"}],"text":"Simple Message","notifyContentType":"application/json","intermediateReport":true,"notifyUrl":"https://localhost/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered","transliteration":"COLOMBIAN"}]}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)

	var transChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
			configTransliteration:  "COLOMBIAN",
		})

	RunChannelSendTestCases(t, transChannel, newHandler(), transSendTestCases, nil)
}
