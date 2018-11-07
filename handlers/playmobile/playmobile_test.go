package playmobile

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "2020", "UZ", nil),
}

var (
	receiveURL = "/c/pm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validMessage = `{
		"messages": [
			{
				"recipient": "1122",
				"message-id": "2018-10-26-09-27-34",
				"sms": {
					"originator": "99999999999",
					"content": {
						"text": "Incoming Valid Message"
					}
				}
			}
		]
	}`

	missingRecipient = `{
		"messages": [
			{
				"message-id": "2018-10-26-09-27-34",
				"sms": {
					"originator": "1122",
					"content": {
						"text": "Message from Paul"
					}
				}
			}
		]
	}`

	missingMessageID = `{
		"messages": [
			{
				"recipient": "99999999999",
				"sms": {
					"originator": "1122",
					"content": {
						"text": "Message from Paul"
					}
				}
			}
		]
	}`
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validMessage, Response: "Message Accepted", 
		Status: 200,
		Text: Sp("Incoming Valid Message"),
		URN: Sp("tel:99999999999")},
	{Label: "Receive Missing Recipient", URL: receiveURL, Data: missingRecipient, Response: "missing required fields recipient, message-id, originator or content",
		Status: 400},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message",
		URN: "tel:99999999999",
		Status: "W",
		ExternalID: "",
		ResponseBody: "Request is received",
		ResponseStatus: 200,
		RequestBody: `{"messages":[{"recipient":"99999999999","message-id":"10","sms":{"originator":"2020","content":{"text":"Simple Message"}}}]}`,
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!",
		URN: "tel:+18686846481",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W",
		ExternalID: "",
		ResponseBody: validMessage,
		ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Invalid JSON Response",
		Text: "Error Sending",
		URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 400,
		ResponseBody:   "not json",
		SendPrep:       setSendURL},
	{Label: "Missing Message ID",
		Text: missingMessageID,
		URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 400,
		ResponseBody:   "{}",
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "2020", "UZ",
		map[string]interface{}{
			"auth_basic_password": "Password",
			"auth_basic_username": "Username",
			"phone_sender": "2020",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
