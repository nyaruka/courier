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

	statusURL = "/c/bl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive  = "id=12345678&message=Msg&sourceaddr=254791541111"
	missingNumber = "id=12345679&message=Msg"

	validStatus   = "id=12345&status=1"
	invalidStatus = "id=12345&status=12"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111")},
	{Label: "Receive Missing Number", URL: receiveURL, Data: missingNumber, Status: 400, Response: ""},
	{Label: "Status No params", URL: statusURL, Data: "", Status: 400, Response: ""},
	{Label: "Status invalid params", URL: statusURL, Data: invalidStatus, Status: 400, Response: ""},
	{Label: "Status valid", URL: statusURL, Data: validStatus, Status: 200, Response: ""},
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
		Status:         "W",
		ResponseBody:   `status=0&msgid=123`,
		ResponseStatus: 200,
		URLParams: map[string]string{
			"username":   "user1",
			"password":   "pass1",
			"apikey":     "api-key",
			"sourceaddr": "2020",
			"destaddr":   "250788383383",
			"dlr":        "1",
			"dlrid":      "10",
			"message":    "Simple Message ☺\nhttps://foo.bar/image.jpg",
		},
		SendPrep: setSendURL},
	{Label: "Error status 403",
		Text: "Error Response", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `status=9&msgid=123`, ResponseStatus: 403,
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
