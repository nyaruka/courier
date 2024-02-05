package tembachat

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Message with text",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "65vbbDAQCdPdEWlEhDGy4utO", "secret": "sesame", "events": [{"type": "msg_in", "msg": {"text": "Join"}}]}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Events Handled",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "webchat:65vbbDAQCdPdEWlEhDGy4utO",
	},
	{
		Label:                "Message with invalid chat ID",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "xxxxx", "secret": "sesame", "events": [{"type": "msg_in", "msg": {"text": "Join"}}]}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid webchat id: xxxxx",
	},
	{
		Label:                "Chat started event",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "65vbbDAQCdPdEWlEhDGy4utO", "secret": "sesame", "events": [{"type": "chat_started"}]}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Events Handled",
		ExpectedEvents:       []ExpectedEvent{{Type: courier.EventTypeNewConversation, URN: "webchat:65vbbDAQCdPdEWlEhDGy4utO"}},
	},
	{
		Label:                "Chat started event with invalid chat ID",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "xxxxx", "secret": "sesame", "events": [{"type": "chat_started"}]}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid webchat id: xxxxx",
	},
	{
		Label:                "Msg status update",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "65vbbDAQCdPdEWlEhDGy4utO", "secret": "sesame", "events": [{"type": "msg_status", "status": {"msg_id": 10, "status": "sent"}}]}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Events Handled",
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 10, Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Missing fields",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"foo": "bar"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'ChatID' failed on the 'required' tag",
	},
	{
		Label:                "Incorrect channel secret",
		URL:                  "/c/twc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		Data:                 `{"chat_id": "65vbbDAQCdPdEWlEhDGy4utO", "secret": "xxxxx", "events": [{"type": "msg_in", "msg": {"text": "Join"}}]}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "secret incorrect",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", map[string]any{"secret": "sesame"}),
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
			{
				Params: url.Values{"channel": []string{"8eb23e93-5ecb-45ba-b726-3b064e0c56ab"}},
				Body:   `{"chat_id":"65vbbDAQCdPdEWlEhDGy4utO","secret":"sesame","msg":{"id":10,"text":"Simple message ☺","origin":"flow"}}`,
			},
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Chat message",
		MsgText:            "Simple message ☺",
		MsgURN:             "webchat:65vbbDAQCdPdEWlEhDGy4utO",
		MsgUserID:          123,
		MockResponseBody:   `{"status": "queued"}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{"channel": []string{"8eb23e93-5ecb-45ba-b726-3b064e0c56ab"}},
				Body:   `{"chat_id":"65vbbDAQCdPdEWlEhDGy4utO","secret":"sesame","msg":{"id":10,"text":"Simple message ☺","origin":"flow","user_id":123}}`,
			},
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
			{
				Params: url.Values{"channel": []string{"8eb23e93-5ecb-45ba-b726-3b064e0c56ab"}},
				Body:   `{"chat_id":"65vbbDAQCdPdEWlEhDGy4utO","secret":"sesame","msg":{"id":10,"text":"Error message","origin":"flow"}}`,
			},
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TWC", "", "", map[string]any{"secret": "sesame"})

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"sesame"}, nil)
}
