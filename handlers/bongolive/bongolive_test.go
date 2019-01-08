package bongolive

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE", nil),
}

var (
	receiveURL = "/c/bl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive  = "message=Msg&org=254791541111"
	missingNumber = "message=Msg"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111")},
	{Label: "Receive Missing Number", URL: receiveURL, Data: missingNumber, Status: 400, Response: "field 'from' required"},
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
		Status: "W",
		ResponseBody: `<response>
		<code>1</code>
		<text>MT is successfully sent</text>
</response>`,
		ResponseStatus: 200,
		URLParams: map[string]string{
			"username":   "user1",
			"password":   "pass1",
			"apikey":     "api-key",
			"sendername": "2020",
			"destnum":    "+250788383383",
			"message":    "Simple Message ☺\nhttps://foo.bar/image.jpg",
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
		ResponseBody: `<response><code>-1</code><text>failure</text></response>`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Code not Int",
		Text: "Error Response", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `<response><code>foo</code><text>failure</text></response>`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `Bad Gateway`, ResponseStatus: 501,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE",
		map[string]interface{}{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			courier.ConfigAPIKey:   "api-key",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
