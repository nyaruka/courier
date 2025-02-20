package discord

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var testChannels = []courier.Channel{
	test.NewMockChannel("bac782c2-7aeb-4389-92f5-97887744f573", "DS", "discord", "US", []string{urns.Discord.Prefix}, map[string]any{courier.ConfigSendAuthorization: "sesame", courier.ConfigSendURL: "http://example.com/discord/rp/send"}),
}

var testCases = []IncomingTestCase{
	{
		Label:              "Receive Message",
		URL:                "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive",
		Data:               `from=694634743521607802&text=hello`,
		ExpectedRespStatus: 200,
		ExpectedMsgText:    Sp("hello"),
		ExpectedURN:        "discord:694634743521607802",
	},
	{
		Label:               "Receive Message with attachment",
		URL:                 "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive",
		Data:                `from=694634743521607802&text=hello&attachments=https://test.test/foo.png`,
		ExpectedRespStatus:  200,
		ExpectedMsgText:     Sp("hello"),
		ExpectedURN:         "discord:694634743521607802",
		ExpectedAttachments: []string{"https://test.test/foo.png"},
	},
	{
		Label:                "Invalid ID",
		URL:                  "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive",
		Data:                 `from=somebody&text=hello`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Garbage Body",
		URL:                  "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive",
		Data:                 `sdfaskdfajsdkfajsdfaksdf`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Missing Text",
		URL:                  "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/receive",
		Data:                 `from=694634743521607802`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
	{
		Label:                "Message Sent Handler",
		URL:                  "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/sent/",
		Data:                 `id=12345`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusSent}},
	},
	{
		Label:              "Message Sent Handler Garbage",
		URL:                "/c/ds/bac782c2-7aeb-4389-92f5-97887744f573/sent/",
		Data:               `nothing`,
		ExpectedRespStatus: 400,
	},
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:   "Simple Send",
		MsgText: "Hello World",
		MsgURN:  "discord:694634743521607802",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/discord/rp/send": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/discord/rp/send",
				Body: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":[],"quick_replies":[]}`,
			},
		},
	},
	{
		Label:          "Attachment",
		MsgText:        "Hello World",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgURN:         "discord:694634743521607802",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/discord/rp/send": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/discord/rp/send",
				Body: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":["https://foo.bar/image.jpg"],"quick_replies":[]}`,
			},
		},
	},
	{
		Label:           "Attachement and quick replies",
		MsgText:         "Hello World",
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgQuickReplies: []courier.QuickReply{{Text: "hello"}, {Text: "world"}},
		MsgURN:          "discord:694634743521607802",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/discord/rp/send": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/discord/rp/send",
				Body: `{"id":"10","text":"Hello World","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":["https://foo.bar/image.jpg"],"quick_replies":["hello","world"]}`,
			},
		},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Sending",
		MsgURN:  "discord:694634743521607802",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/discord/rp/send": {
				httpx.NewMockResponse(400, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/discord/rp/send",
				Body: `{"id":"10","text":"Error Sending","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":[],"quick_replies":[]}`,
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Connection Error",
		MsgText: "Error",
		MsgURN:  "discord:694634743521607802",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/discord/rp/send": {
				httpx.NewMockResponse(500, nil, []byte(``)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/discord/rp/send",
				Body: `{"id":"10","text":"Error","to":"694634743521607802","channel":"bac782c2-7aeb-4389-92f5-97887744f573","attachments":[],"quick_replies":[]}`,
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newHandler(), sendTestCases, []string{"sesame"}, nil)
}
