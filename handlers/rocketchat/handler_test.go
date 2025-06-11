package rocketchat

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/rc/" + channelUUID + "/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "RC", "1234", "",
		[]string{urns.RocketChat.Prefix},
		map[string]any{
			configBaseURL:     "https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c",
			configSecret:      "123456789",
			configBotUsername: "rocket.cat",
		},
	),
}

const emptyMsg = `{
	"user": {
		"urn": "direct:john.doe",
		"username": "john.doe",
		"full_name": "John Doe"
	}
}`

const helloMsg = `{
	"user": {
		"urn": "direct:john.doe",
		"username": "john.doe",
		"full_name": "John Doe"
	},
	"text": "Hello World"
}`

const attachmentMsg = `{
	"user": {
		"urn": "livechat:onrMgdKbpX9Qqtvoi",
		"full_name": "John Doe"
	},
	"attachments": [{"type": "image/jpg", "url": "https://link.to/image.jpg"}]
}`

var testCases = []IncomingTestCase{
	{
		Label: "Receive Hello Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:                 helloMsg,
		ExpectedURN:          "rocketchat:direct:john.doe#john.doe",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
	},
	{
		Label: "Receive Attachment Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:                 attachmentMsg,
		ExpectedURN:          "rocketchat:livechat:onrMgdKbpX9Qqtvoi",
		ExpectedAttachments:  []string{"https://link.to/image.jpg"},
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
	},
	{
		Label: "Don't Receive Empty Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:                 emptyMsg,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "no text or attachment",
	},
	{
		Label: "Invalid Authorization",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "123456789",
		},
		Data:                 emptyMsg,
		ExpectedRespStatus:   401,
		ExpectedBodyContains: "invalid Authorization header",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "rocketchat:direct:john.doe#john.doe",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(201, nil, []byte(`{"id":"iNKE8a6k6cjbqWhWd"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		}},
		ExpectedExtIDs: []string{"iNKE8a6k6cjbqWhWd"},
	},
	{
		Label:          "Send Attachment",
		MsgURN:         "rocketchat:livechat:onrMgdKbpX9Qqtvoi",
		MsgAttachments: []string{"application/pdf:https://link.to/attachment.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(201, nil, []byte(`{"id":"iNKE8a6k6cjbqWhWd"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"livechat:onrMgdKbpX9Qqtvoi","bot":"rocket.cat","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		}},
		ExpectedExtIDs: []string{"iNKE8a6k6cjbqWhWd"},
	},
	{
		Label:          "Send Text And Attachment",
		MsgURN:         "rocketchat:direct:john.doe",
		MsgText:        "Simple Message",
		MsgAttachments: []string{"application/pdf:https://link.to/attachment.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(201, nil, []byte(`{"id":"iNKE8a6k6cjbqWhWd"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		}},
		ExpectedExtIDs: []string{"iNKE8a6k6cjbqWhWd"},
	},
	{
		Label:   "Unexcepted status response",
		MsgText: "Simple Message",
		MsgURN:  "rocketchat:direct:john.doe#john.doe",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(400, nil, []byte(`Error`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Connection Error",
		MsgText: "Simple Message",
		MsgURN:  "rocketchat:direct:john.doe#john.doe",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(500, nil, []byte(`Error`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
	{
		Label:   "Response Unexpected",
		MsgText: "Simple Message",
		MsgURN:  "rocketchat:direct:john.doe#john.doe",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://my.rocket.chat/api/apps/public/684202ed-1461-4983-9ea7-fde74b15026c/message": {
				httpx.NewMockResponse(201, nil, []byte(`{"missing":"0"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		}},
		ExpectedError: courier.ErrResponseContent,
	},
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newHandler(), sendTestCases, []string{"123456789"}, nil)
}
