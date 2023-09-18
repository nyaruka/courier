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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US", map[string]any{"username": "Username", "password": "Password"}),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 "mobile=%2B2349067554729&message=Join",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive No Message",
		URL:                  receiveURL,
		Data:                 "mobile=%2B2349067554729",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive invalid URN",
		URL:                  receiveURL,
		Data:                 "mobile=MTN&message=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive No Params",
		URL:                  receiveURL,
		Data:                 "none",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
	{
		Label:                "Receive No Sender",
		URL:                  receiveURL,
		Data:                 "message=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"id": "1002"}]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "Simple Message", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"id": "1002"}]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "☺", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{ "id": "1002" }]`, MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg", "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "E",
		MockResponseBody:  `{ "error": "failed" }`, MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"content": `Error Message`, "mobile": "250788383383", "pass": "Password", "user": "Username"},
		SendPrep:           setSendURL},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		map[string]any{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
