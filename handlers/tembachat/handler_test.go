package tembachat

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "message", "message": {"identifier": "65vbbDAQCdPdEWlEhDGy4utO", "text": "Join"}}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "webchat:65vbbDAQCdPdEWlEhDGy4utO",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "message", "message": {"identifier": "xxxxx", "text": "Join"}}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid webchat id: xxxxx",
	},
	{
		Label:                "Missing fields",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"foo": "message"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'Type' failed on the 'required' tag",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	defaultSendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple message ☺",
		MsgURN:              "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MockResponseBody:    `{"status": "queued"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"identifier":"65vbbDAQCdPdEWlEhDGy4utO","text":"Simple message ☺","origin":"flow"}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error message",
		MsgURN:              "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MockResponseBody:    `{"error": "boom"}`,
		MockResponseStatus:  400,
		ExpectedRequestBody: `{"identifier":"65vbbDAQCdPdEWlEhDGy4utO","text":"Error message","origin":"flow"}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", nil)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil, nil)
}
