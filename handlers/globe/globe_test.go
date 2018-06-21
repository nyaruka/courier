package globe

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL = "/c/gl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	validMessage = `
	{
		"inboundSMSMessageList":{
			"inboundSMSMessage":[
			   {
				  "dateTime":"Fri Nov 22 2013 12:12:13 GMT+0000 (UTC)",
				  "destinationAddress":"tel:21581234",
				  "messageId":null,
				  "message":"hello world",
				  "resourceURL":null,
				  "senderAddress":"tel:+639171234567"
			   }
			 ],
			 "numberOfMessagesInThisBatch":1,
			 "resourceURL":null,
			 "totalNumberOfPendingMessages":null
		 }
	}
	`

	invalidURN = `
	{
		"inboundSMSMessageList":{
			"inboundSMSMessage":[
			   {
				  "dateTime":"Fri Nov 22 2013 12:12:13 GMT+0000 (UTC)",
				  "destinationAddress":"tel:21581234",
				  "messageId":null,
				  "message":"hello world",
				  "resourceURL":null,
				  "senderAddress":"tel:MTN"
			   }
			 ],
			 "numberOfMessagesInThisBatch":1,
			 "resourceURL":null,
			 "totalNumberOfPendingMessages":null
		 }
	}
	`

	noMessages = `
	{
		"inboundSMSMessageList":{
			"inboundSMSMessage":[],
			"numberOfMessagesInThisBatch":1,
			"resourceURL":null,
			"totalNumberOfPendingMessages":null
		 }
	}
	`

	invalidSender = `
	{
		"inboundSMSMessageList":{
			"inboundSMSMessage":[
			   {
				  "dateTime":"Fri Nov 22 2013 12:12:13 GMT+0000 (UTC)",
				  "destinationAddress":"tel:21581234",
				  "messageId":null,
				  "message":"hello world",
				  "resourceURL":null,
				  "senderAddress":"notvalid"
			   }
			 ],
			 "numberOfMessagesInThisBatch":1,
			 "resourceURL":null,
			 "totalNumberOfPendingMessages":null
		 }
	}
	`

	invalidDate = `
	{
		"inboundSMSMessageList":{
			"inboundSMSMessage":[
			   {
				  "dateTime":"Zed Nov 22 2013 12:12:13 GMT+0000 (UTC)",
				  "destinationAddress":"tel:21581234",
				  "messageId":null,
				  "message":"hello world",
				  "resourceURL":null,
				  "senderAddress":"tel:+639171234567"
			   }
			 ],
			 "numberOfMessagesInThisBatch":1,
			 "resourceURL":null,
			 "totalNumberOfPendingMessages":null
		 }
	}
	`

	invalidJSON = `notjson`
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: validMessage, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+639171234567"), Date: Tp(time.Date(2013, 11, 22, 12, 12, 13, 0, time.UTC))},
	{Label: "No Messages", URL: receiveURL, Data: noMessages, Status: 200, Response: "Ignored"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Invalid Sender", URL: receiveURL, Data: invalidSender, Status: 400, Response: "invalid 'senderAddress' parameter"},
	{Label: "Invalid Date", URL: receiveURL, Data: invalidDate, Status: 400, Response: "parsing time"},
	{Label: "Invalid JSON", URL: receiveURL, Data: invalidJSON, Status: 400, Response: "unable to parse request JSON"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "?%s"
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		RequestBody: `{"address":"250788383383","message":"Simple Message","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		RequestBody: `{"address":"250788383383","message":"☺","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		RequestBody: `{"address":"250788383383","message":"My pic!\nhttps://foo.bar/image.jpg","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `[{"Response": "101"}]`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US",
		map[string]interface{}{
			"app_id":     "12345",
			"app_secret": "mysecret",
			"passphrase": "opensesame",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), sendTestCases, nil)
}
