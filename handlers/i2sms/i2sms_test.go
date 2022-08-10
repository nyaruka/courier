package i2sms

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US", nil),
}

var (
	receiveURL = "/c/i2/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive  = "message=Msg&mobile=254791541111"
	missingNumber = "message=Msg"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111")},
	{Label: "Receive Missing Number", URL: receiveURL, Data: missingNumber, Status: 400, Response: "required field 'mobile'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message ☺", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedStatus: "W", ExpectedExternalID: "5b8fc97d58795484819426",
		MockResponseBody: `{"result":{"session_id":"5b8fc97d58795484819426"}, "error_code": "00", "error_desc": "Success"}`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"action":  "send_single",
			"mobile":  "250788383383",
			"message": "Simple Message ☺\nhttps://foo.bar/image.jpg",
			"channel": "hash123",
		},
		SendPrep: setSendURL},
	{Label: "Invalid JSON",
		MsgText: "Invalid XML", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: `not json`, MockResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Response",
		MsgText: "Error Response", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "F",
		MockResponseBody: `{"result":{}, "error_code": "10", "error_desc": "Failed"}`, MockResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: `Bad Gateway`, MockResponseStatus: 501,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configChannelHash:      "hash123",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
