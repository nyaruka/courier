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
		Text:  "Simple Message", URN: "slack:12345",
		Status: "W", ExternalID: "123",
		ResponseBody:   `{"ok":true,"channel":"C0123ABCDEF","ts":"1652391855.466329","message":{"bot_id":"B03F2F82B4N","type":"message","text":"hello","user":"U03F94B4UMQ","ts":"1652391855.466329","app_id":"A03FBEC4NE8","team":"T03CN5KTA6S","bot_profile":{"id":"B03F2F82B4N","app_id":"A03FBEC4NE8","name":"Test Bot","icons":{"image_36":"https:\/\/a.slack-edge.com\/80588\/img\/plugins\/app\/bot_36.png","image_48":"https:\/\/a.slack-edge.com\/80588\/img\/plugins\/app\/bot_48.png","image_72":"https:\/\/a.slack-edge.com\/80588\/img\/plugins\/app\/service_72.png"},"deleted":false,"updated":1652389333,"team_id":"T03CN5KTA6S"},"blocks":[{"type":"rich_text","block_id":"jv9h","elements":[{"type":"rich_text_section","elements":[{"type":"text","text":"hello"}]}]}]}}`,
		ResponseStatus: 200,
		RequestBody:    `{"chanel": "C0123ABCDEF", "text": "Hello"}`,
		SendPrep:       setSendUrl,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SL", "2022", "US",
		map[string]interface{}{
			courier.ConfigAuthToken: "xoxb-123456789...",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
