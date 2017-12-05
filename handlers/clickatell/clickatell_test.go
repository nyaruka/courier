package clickatell

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSendURL takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "Unicode ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "Unicode ☺", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "1", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "I need to keep adding more things to make it work", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Error Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Invalid Token",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "Invalid API token", ResponseStatus: 401,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
			courier.ConfigAPIID:    "API-ID",
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
