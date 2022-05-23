package slack

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/sl/" + channelUUID + "/receive/"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel(channelUUID, "SL", "2022", "US", map[string]interface{}{"bot_token": "xoxb-abc123"}),
}

const helloMsg = `{
	"token": "one-long-verification-token",
	"team_id": "T061EG9R6",
	"api_app_id": "A0PNCHHK2",
	"event": {
			"type": "message",
			"channel": "C0123ABCDEF",
			"user": "U0123ABCDEF",
			"text": "Hello World!",
			"ts": "1355517523.000005",
			"event_ts": "1355517523.000005",
			"channel_type": "channel"
	},
	"type": "event_callback",
	"authed_teams": [
			"T061EG9R6"
	],
	"event_id": "Ev0PV52K21",
	"event_time": 1355517523
}`

func setSendUrl(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var testCases = []ChannelHandleTestCase{
	{
		Label:      "Receive Hello Msg",
		URL:        receiveURL,
		Headers:    map[string]string{},
		Data:       helloMsg,
		URN:        Sp("slack:U0123ABCDEF"),
		Text:       Sp("Hello World!"),
		Status:     200,
		Response:   "Accepted",
		ExternalID: Sp("Ev0PV52K21"),
	},
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

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannels[0], newHandler(), defaultSendTestCases, nil)
}

func TestVerification(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), []ChannelHandleTestCase{
		{Label: "Valid token", URL: receiveURL, Status: 200,
			Data:     `{"token":"xoxb-abc123","challenge":"challenge123","type":"url_verification"}`,
			Headers:  map[string]string{"content-type": "text/plain"},
			Response: "challenge123", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		},
		{Label: "Invalid token", URL: receiveURL, Status: 403,
			Data:    `{"token":"abc321","challenge":"challenge123","type":"url_verification"}`,
			Headers: map[string]string{"content-type": "text/plain"},
		},
	})
}
