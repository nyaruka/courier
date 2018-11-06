package i2sms

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US", nil),
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
		Text: "Simple Message ☺", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "5b8fc97d58795484819426",
		ResponseBody: `{"result":{"session_id":"5b8fc97d58795484819426"}, "error_code": "00", "error_desc": "Success"}`, ResponseStatus: 200,
		PostParams: map[string]string{
			"action":  "send_single",
			"mobile":  "250788383383",
			"message": "Simple Message ☺\nhttps://foo.bar/image.jpg",
			"channel": "hash123",
		},
		SendPrep: setSendURL},
	{Label: "Invalid JSON",
		Text: "Invalid XML", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `not json`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Response",
		Text: "Error Response", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{"result":{}, "error_code": "10", "error_desc": "Failed"}`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `Bad Gateway`, ResponseStatus: 501,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "I2", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configChannelHash:      "hash123",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
