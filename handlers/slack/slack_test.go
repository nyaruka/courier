package slack

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SL", "2022", "US", map[string]interface{}{"auth_token": "abc123"}),
}

var helloMsg = `{
	
}`

func setSendUrl(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label: "Plain Send",
		Text:  "Simple Message", URN: "slack:C0123ABCDEF",
		Status:         "W",
		ResponseBody:   `{"ok":true,"channel":"C0123ABCDEF"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"C0123ABCDEF","text":"Simple Message"}`,
		SendPrep:       setSendUrl,
	},
	{
		Label: "Unicode Send",
		Text:  "☺", URN: "slack:U0123ABCDEF",
		Status:         "W",
		ResponseBody:   `{"ok":true,"channel":"U0123ABCDEF"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"U0123ABCDEF","text":"☺"}`,
		SendPrep:       setSendUrl,
	},
	{
		Label: "Send Text Error",
		Text:  "Hello", URN: "slack:U0123ABCDEF",
		Status:         "E",
		ResponseBody:   `{"ok":false,"error":"invalid_auth"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"U0123ABCDEF","text":"Hello"}`,
		SendPrep:       setSendUrl,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SL", "2022", "US",
		map[string]interface{}{
			configBotToken: "xoxb-123456789...",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
