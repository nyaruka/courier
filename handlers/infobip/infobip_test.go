package infobip

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US", nil),
}

var receiveURL = "/c/ib/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: helloMsg, Status: 200, Response: "Accepted",
		Text: Sp("QUIZ Correct answer is Paris"), URN: Sp("tel:+385916242493"), External: Sp("817790313235066447"), Date: Tp(time.Date(2016, 10, 06, 9, 28, 39, 220000000, time.UTC))},
	{Label: "Receive missing results key", URL: receiveURL, Data: missingResults, Status: 400, Response: "validation for 'Results' failed"},
	{Label: "Receive missing results key", URL: receiveURL, Data: missingText, Status: 400, Response: "validation for 'Text' failed"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
