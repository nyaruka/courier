package arabiacell

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US", nil),
}

var (
	receiveURL = "/c/ac/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive  = "B=Msg&M=254791541111"
	missingNumber = "B=Msg"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111")},
	{Label: "Receive Missing Number", URL: receiveURL, Data: missingNumber, Status: 400, Response: "required field 'M'"},
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
		Status: "W", ExternalID: "external1",
		ResponseBody: `<response>
		<code>204</code>
		<text>MT is successfully sent</text>
		<message_id>external1</message_id>
</response>`, ResponseStatus: 200,
		PostParams: map[string]string{
			"userName":      "user1",
			"password":      "pass1",
			"handlerType":   "send_msg",
			"serviceId":     "service1",
			"msisdn":        "+250788383383",
			"messageBody":   "Simple Message ☺\nhttps://foo.bar/image.jpg",
			"chargingLevel": "0",
		},
		SendPrep: setSendURL},
	{Label: "Invalid XML",
		Text: "Invalid XML", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `not xml`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Response",
		Text: "Error Response", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `<response><code>501</code><text>failure</text><message_id></message_id></response>`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `Bad Gateway`, ResponseStatus: 501,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configServiceID:        "service1",
			configChargingLevel:    "0",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
