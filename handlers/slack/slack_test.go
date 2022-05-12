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
		ResponseBody:   `{}`,
		ResponseStatus: 200,
		PostParams:     map[string]string{},
		SendPrep:       setSendUrl,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SL", "2022", "US",
		map[string]interface{}{courier.ConfigAuthToken: "auth_token"})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
