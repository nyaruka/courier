package smscentral

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveValidMessage = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=%2B2349067554729&message=Join"
	receiveNoParams     = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveNoSender     = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?message=Join"
	receiveNoMessage    = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=%2B2349067554729"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US", map[string]interface{}{"username": "Username", "password": "Password"}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'message' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "empty", Status: 400, Response: "field 'mobile' required"},
	{Label: "Receive No Message", URL: receiveNoMessage, Data: "empty", Status: 400, Response: "field 'message' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), handleTestCases)
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
		Error:      "received non 200 status: 401",
		PostParams: map[string]string{"content": `Error Message`, "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
