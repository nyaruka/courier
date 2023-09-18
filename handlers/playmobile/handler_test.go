package playmobile

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ", map[string]any{
		"incoming_prefixes": []string{"abc", "DE"},
	}),
}

var (
	receiveURL = "/c/pm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = `<sms-request><message id="1107962" msisdn="998999999999" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">SMS Response Accepted</content>
	</message></sms-request>`

	invalidReceive = `<sms-request><message id="" msisdn="" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">SMS Response Accepted</content>
	</message></sms-request>`

	noMessages = `<sms-request></sms-request>`

	receiveWithPrefix = `<sms-request><message id="1107962" msisdn="998999999999" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">abc SMS Response Accepted</content>
	<content type="text/plain">aBc SMS Response Accepted</content>
	<content type="text/plain">ABCSMS Response Accepted</content>
	<content type="text/plain">de SMS Response Accepted</content>
	<content type="text/plain">DESMS Response Accepted</content>
	</message></sms-request>`

	receiveWithPrefixOnly = `<sms-request><message id="1107962" msisdn="998999999999" submit-date="2016-11-22 15:10:32">
	<content type="text/plain">abc </content>
	</message></sms-request>`

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

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedBodyContains: "Accepted",
		ExpectedRespStatus:   200,
		ExpectedMsgText:      Sp("SMS Response Accepted"),
		ExpectedURN:          "tel:+998999999999",
	},
	{
		Label:                "Receive Missing MSISDN",
		URL:                  receiveURL,
		Data:                 invalidReceive,
		ExpectedBodyContains: "missing required fields msidsn or id",
		ExpectedRespStatus:   400,
	},
	{
		Label:                "No Messages",
		URL:                  receiveURL,
		Data:                 noMessages,
		ExpectedBodyContains: "no messages, ignored",
		ExpectedRespStatus:   200,
	},
	{
		Label:                "Invalid XML",
		URL:                  receiveURL,
		Data:                 `<>`,
		ExpectedBodyContains: "",
		ExpectedRespStatus:   400,
	},
	{
		Label:                "Receive With Prefix",
		URL:                  receiveURL,
		Data:                 receiveWithPrefix,
		ExpectedBodyContains: "Accepted",
		ExpectedRespStatus:   200,
		ExpectedMsgText:      Sp("SMS Response Accepted"),
		ExpectedURN:          "tel:+998999999999",
	},
	{
		Label:                "Receive With Prefix Only",
		URL:                  receiveURL,
		Data:                 receiveWithPrefixOnly,
		ExpectedBodyContains: "no text",
		ExpectedRespStatus:   400,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL + "?%s"
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:99999999999",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		MockResponseBody:    "Request is received",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messages":[{"recipient":"99999999999","message-id":"10","sms":{"originator":"1122","content":{"text":"Simple Message"}}}]}`,
		SendPrep:            setSendURL},
	{Label: "Long Send",
		MsgText:             "This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, now, I need to keep adding more things to make it work",
		MsgURN:              "tel:99999999999",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "",
		MockResponseBody:    "Request is received",
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messages":[{"recipient":"99999999999","message-id":"10.2","sms":{"originator":"1122","content":{"text":"I need to keep adding more things to make it work"}}}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+18686846481",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "",
		MockResponseBody:   validMessage,
		MockResponseStatus: 200,
		SendPrep:           setSendURL},
	{Label: "Invalid JSON Response",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "E",
		MockResponseStatus: 400,
		MockResponseBody:   "not json",
		SendPrep:           setSendURL},
	{Label: "Missing Message ID",
		MsgText:            missingMessageID,
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "E",
		MockResponseStatus: 400,
		MockResponseBody:   "{}",
		SendPrep:           setSendURL},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
		map[string]any{
			"password":  "Password",
			"username":  "Username",
			"shortcode": "1122",
			"base_url":  "http://91.204.239.42",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)
}
