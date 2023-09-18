package globe

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

const (
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
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US", nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 validMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+639171234567",
		ExpectedDate:         time.Date(2013, 11, 22, 12, 12, 13, 0, time.UTC),
	},
	{
		Label:                "No Messages",
		URL:                  receiveURL,
		Data:                 noMessages,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Ignored",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Invalid Sender",
		URL:                  receiveURL,
		Data:                 invalidSender,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid 'senderAddress' parameter",
	},
	{
		Label:                "Invalid Date",
		URL:                  receiveURL,
		Data:                 invalidDate,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "parsing time",
	},
	{
		Label:                "Invalid JSON",
		URL:                  receiveURL,
		Data:                 `notjson`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request JSON",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL + "?%s"
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `[{"Response": "0"}]`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"address":"250788383383","message":"Simple Message","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    `[{"Response": "0"}]`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"address":"250788383383","message":"☺","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `[{"Response": "0"}]`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"address":"250788383383","message":"My pic!\nhttps://foo.bar/image.jpg","passphrase":"opensesame","app_id":"12345","app_secret":"mysecret"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `[{"Response": "101"}]`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US",
		map[string]any{
			"app_id":     "12345",
			"app_secret": "mysecret",
			"passphrase": "opensesame",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), sendTestCases, []string{"mysecret", "opensesame"}, nil)
}
