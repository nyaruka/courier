package rocketchat

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/rc/" + channelUUID + "/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "RC", "1234", "",
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

var testCases = []handlers.IncomingTestCase{
	{
		Label: "Receive Hello Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:                 helloMsg,
		ExpectedURN:          "rocketchat:direct:john.doe#john.doe",
		ExpectedMsgText:      handlers.Sp("Hello World"),
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
	handlers.RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	handlers.RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	c.(*test.MockChannel).SetConfig(configBaseURL, s.URL)
}

var sendTestCases = []handlers.OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "rocketchat:direct:john.doe#john.doe",
		ExpectedMsgStatus:   "S",
		ExpectedRequestBody: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		MockResponseStatus:  201,
		MockResponseBody:    `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExpectedExternalID:  "iNKE8a6k6cjbqWhWd",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgURN:              "rocketchat:livechat:onrMgdKbpX9Qqtvoi",
		MsgAttachments:      []string{"application/pdf:https://link.to/attachment.pdf"},
		ExpectedMsgStatus:   "S",
		ExpectedRequestBody: `{"user":"livechat:onrMgdKbpX9Qqtvoi","bot":"rocket.cat","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		MockResponseStatus:  201,
		MockResponseBody:    `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExpectedExternalID:  "iNKE8a6k6cjbqWhWd",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Text And Attachment",
		MsgURN:              "rocketchat:direct:john.doe",
		MsgText:             "Simple Message",
		MsgAttachments:      []string{"application/pdf:https://link.to/attachment.pdf"},
		ExpectedMsgStatus:   "S",
		ExpectedRequestBody: `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		MockResponseStatus:  201,
		MockResponseBody:    `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExpectedExternalID:  "iNKE8a6k6cjbqWhWd",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	handlers.RunOutgoingTestCases(t, testChannels[0], newHandler(), sendTestCases, []string{"123456789"}, nil)
}
