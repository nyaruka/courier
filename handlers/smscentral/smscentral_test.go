package smscentral

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL          = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	receiveValidMessage = "mobile=%2B2349067554729&message=Join"
	invalidURN          = "mobile=MTN&message=Join"
	receiveNoMessage    = "mobile=%2B2349067554729"
	receiveNoParams     = "none"
	receiveNoSender     = "message=Join"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US", map[string]interface{}{"username": "Username", "password": "Password"}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive No Message", URL: receiveURL, Data: receiveNoMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("tel:+2349067554729")},
	{Label: "Receive invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveURL, Data: receiveNoParams, Status: 400, Response: "field 'mobile' required"},
	{Label: "Receive No Sender", URL: receiveURL, Data: receiveNoSender, Status: 400, Response: "field 'mobile' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"id": "1002"}]`, ResponseStatus: 200,
		PostParams: map[string]string{"content": "Simple Message", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:   setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"id": "1002"}]`, ResponseStatus: 200,
		PostParams: map[string]string{"content": "☺", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `[{ "id": "1002" }]`, ResponseStatus: 200,
		PostParams: map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		PostParams: map[string]string{"content": `Error Message`, "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
