package rocketchat

import (
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"net/http/httptest"
	"testing"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/rc/" + channelUUID + "/receive"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "RC", "1234", "",
		map[string]interface{}{
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

var testCases = []ChannelHandleTestCase{
	{
		Label: "Receive Hello Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:     helloMsg,
		URN:      Sp("rocketchat:direct:john.doe#john.doe"),
		Text:     Sp("Hello World"),
		Status:   200,
		Response: "Accepted",
	},
	{
		Label: "Receive Attachment Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:       attachmentMsg,
		URN:        Sp("rocketchat:livechat:onrMgdKbpX9Qqtvoi"),
		Attachment: Sp("https://link.to/image.jpg"),
		Status:     200,
		Response:   "Accepted",
	},
	{
		Label: "Don't Receive Empty Msg",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "Token 123456789",
		},
		Data:     emptyMsg,
		Status:   400,
		Response: "no text or attachment",
	},
	{
		Label: "Invalid Authorization",
		URL:   receiveURL,
		Headers: map[string]string{
			"Authorization": "123456789",
		},
		Data:     emptyMsg,
		Status:   401,
		Response: "invalid Authorization header",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig(configBaseURL, s.URL)
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:          "Plain Send",
		Text:           "Simple Message",
		URN:            "rocketchat:direct:john.doe#john.doe",
		Status:         "S",
		RequestBody:    `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message"}`,
		ResponseStatus: 201,
		ResponseBody:   `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExternalID:     "iNKE8a6k6cjbqWhWd",
		SendPrep:       setSendURL,
	},
	{
		Label:          "Send Attachment",
		URN:            "rocketchat:livechat:onrMgdKbpX9Qqtvoi",
		Attachments:    []string{"application/pdf:https://link.to/attachment.pdf"},
		Status:         "S",
		RequestBody:    `{"user":"livechat:onrMgdKbpX9Qqtvoi","bot":"rocket.cat","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		ResponseStatus: 201,
		ResponseBody:   `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExternalID:     "iNKE8a6k6cjbqWhWd",
		SendPrep:       setSendURL,
	},
	{
		Label:          "Send Text And Attachment",
		URN:            "rocketchat:direct:john.doe",
		Text:           "Simple Message",
		Attachments:    []string{"application/pdf:https://link.to/attachment.pdf"},
		Status:         "S",
		RequestBody:    `{"user":"direct:john.doe","bot":"rocket.cat","text":"Simple Message","attachments":[{"type":"application/pdf","url":"https://link.to/attachment.pdf"}]}`,
		ResponseStatus: 201,
		ResponseBody:   `{"id":"iNKE8a6k6cjbqWhWd"}`,
		ExternalID:     "iNKE8a6k6cjbqWhWd",
		SendPrep:       setSendURL,
	},
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannels[0], newHandler(), sendTestCases, nil)
}
