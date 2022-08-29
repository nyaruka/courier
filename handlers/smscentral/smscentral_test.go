package smscentral

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

const (
	receiveURL = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US", map[string]interface{}{"username": "Username", "password": "Password"}),
}

var handleTestCases = []ChannelHandleTestCase{
	{
		Label:            "Receive Valid Message",
		URL:              receiveURL,
		Data:             "mobile=%2B2349067554729&message=Join",
		ExpectedStatus:   200,
		ExpectedResponse: "Accepted",
		ExpectedMsgText:  Sp("Join"),
		ExpectedURN:      "tel:+2349067554729",
	},
	{
		Label:            "Receive No Message",
		URL:              receiveURL,
		Data:             "mobile=%2B2349067554729",
		ExpectedStatus:   200,
		ExpectedResponse: "Accepted",
		ExpectedMsgText:  Sp(""),
		ExpectedURN:      "tel:+2349067554729",
	},
	{
		Label:            "Receive invalid URN",
		URL:              receiveURL,
		Data:             "mobile=MTN&message=Join",
		ExpectedStatus:   400,
		ExpectedResponse: "phone number supplied is not a number",
	},
	{
		Label:            "Receive No Params",
		URL:              receiveURL,
		Data:             "none",
		ExpectedStatus:   400,
		ExpectedResponse: "field 'mobile' required",
	},
	{
		Label:            "Receive No Sender",
		URL:              receiveURL,
		Data:             "message=Join",
		ExpectedStatus:   400,
		ExpectedResponse: "field 'mobile' required",
	},
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
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: `[{"id": "1002"}]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "Simple Message", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: `[{"id": "1002"}]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "☺", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedStatus:   "W",
		MockResponseBody: `[{ "id": "1002" }]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: `{ "error": "failed" }`, MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"content": `Error Message`, "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
