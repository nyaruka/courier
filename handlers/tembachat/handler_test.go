package tembachat

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Message with text",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "msg_in", "msg": {"identifier": "65vbbDAQCdPdEWlEhDGy4utO", "text": "Join"}}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "webchat:65vbbDAQCdPdEWlEhDGy4utO",
	},
	{
		Label:                "Message with invalid URN",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "msg_in", "msg": {"identifier": "xxxxx", "text": "Join"}}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid webchat id: xxxxx",
	},
	{
		Label:                "Chat started event",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "chat_started", "chat": {"identifier": "65vbbDAQCdPdEWlEhDGy4utO"}}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents:       []ExpectedEvent{{Type: courier.EventTypeNewConversation, URN: "webchat:65vbbDAQCdPdEWlEhDGy4utO"}},
	},
	{
		Label:                "Chat started event with invalid URN",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"type": "chat_started", "chat": {"identifier": "xxxxx"}}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid webchat id: xxxxx",
	},
	{
		Label:                "Missing fields",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"foo": "bar"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'Type' failed on the 'required' tag",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", nil),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	defaultSendURL = s.URL
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:              "Flow message",
		MsgText:            "Simple message ☺",
		MsgURN:             "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MockResponseBody:   `{"status": "queued"}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"identifier":"65vbbDAQCdPdEWlEhDGy4utO","text":"Simple message ☺","origin":"flow"}`},
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Chat message",
		MsgText:            "Simple message ☺",
		MsgURN:             "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MsgUser:            &courier.UserReference{Email: "bob@nyaruka.com", Name: "Bob"},
		MockResponseBody:   `{"status": "queued"}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"identifier":"65vbbDAQCdPdEWlEhDGy4utO","text":"Simple message ☺","origin":"flow","user":{"email":"bob@nyaruka.com","name":"Bob"}}`},
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Error sending",
		MsgText:            "Error message",
		MsgURN:             "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MockResponseBody:   `{"error": "boom"}`,
		MockResponseStatus: 400,
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"identifier":"65vbbDAQCdPdEWlEhDGy4utO","text":"Error message","origin":"flow"}`},
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", nil)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, nil, nil)
}
