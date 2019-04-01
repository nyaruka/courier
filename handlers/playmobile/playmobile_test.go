package playmobile

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ", nil),
}

var (
	receiveURL = "/c/pm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = `<sms-request><message id="1107962" msisdn="998999999999" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">SMS Response Accepted</content>
	</message></sms-request>`

	invalidReceive = `<sms-request><message id="" msisdn="" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">SMS Response Accepted</content>
	</message></sms-request>`

	invalidXML = ``

	noMessages = `<sms-request></sms-request>`

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
	{Label: "Receive Valid",
		URL: receiveURL,
		Data: validReceive,
		Response: "Accepted",
		Status: 200,
		Text: Sp("SMS Response Accepted"),
		URN: Sp("tel:+998999999999")},
	{Label: "Receive Missing MSISDN",
		URL: receiveURL,
		Data: invalidReceive,
		Response: "missing required fields msidsn or id",
		Status: 400},
	{Label: "No Messages",
		URL: receiveURL,
		Data: noMessages,
		Response: "no messages, ignored",
		Status: 200},
	{Label: "Invalid XML",
		URL: receiveURL,
		Data: invalidXML,
		Response: "",
		Status: 405},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "?%s"
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message",
		URN: "tel:99999999999",
		Status: "W",
		ExternalID: "",
		ResponseBody: "Request is received",
		ResponseStatus: 200,
		RequestBody: `{"messages":[{"recipient":"99999999999","message-id":"10","sms":{"originator":"1122","content":{"text":"Simple Message"}}}]}`,
		SendPrep:   setSendURL},
	{Label: "Long Send",
		Text: "This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, now, I need to keep adding more things to make it work",
		URN: "tel:99999999999",
		Status: "W",
		ExternalID: "",
		ResponseBody: "Request is received",
		ResponseStatus: 200,
		RequestBody: `{"messages":[{"recipient":"99999999999","message-id":"10.2","sms":{"originator":"1122","content":{"text":"I need to keep adding more things to make it work"}}}]}`,
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
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
			"shortcode": "1122",
			"base_url": "http://91.204.239.42",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
