package viber

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSend takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"☺","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ResponseStatus: 200,
		ResponseBody: `{"status":0,"status_message":"ok","message_token":4987381194038857789}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"My pic!\nhttps://foo.bar/image.jpg","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Got non-0 response",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "F", ResponseStatus: 200,
		ResponseBody: `{"status":3,"status_message":"InvalidToken"}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Got Invalid JSON response",
		Text: "Simple Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "F", ResponseStatus: 200,
		ResponseBody: `invalidJSON`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Simple Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "viber:xy5/5y6O81+/kbWHpLhBoA==",
		Status: "E", ResponseStatus: 401,
		ResponseBody: `{"status":"5"}`,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"auth_token":"Token","receiver":"xy5/5y6O81+/kbWHpLhBoA==","text":"Error Message","type":"text","tracking_data":"10"}`,
		SendPrep:    setSendURL},
}

var invalidTokenSendTestCases = []ChannelSendTestCase{
	{Label: "Invalid token",
		Error: "invalid auth token config"},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]interface{}{
			courier.ConfigAuthToken: "Token",
		})
	var invalidTokenChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		map[string]interface{}{},
	)
	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
	RunChannelSendTestCases(t, invalidTokenChannel, NewHandler(), invalidTokenSendTestCases)
}
