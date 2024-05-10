package playmobile

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"incoming_prefixes": []string{"abc", "DE"},
		},
	),
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

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:99999999999",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/broker-api/send": {
				httpx.NewMockResponse(200, nil, []byte(`Request is received`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messages":[{"recipient":"99999999999","message-id":"10","sms":{"originator":"1122","content":{"text":"Simple Message"}}}]}`,
		}},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, now, I need to keep adding more things to make it work",
		MsgURN:  "tel:99999999999",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/broker-api/send": {
				httpx.NewMockResponse(200, nil, []byte(`Request is received`)),
				httpx.NewMockResponse(200, nil, []byte(`Request is received`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Body: `{"messages":[{"recipient":"99999999999","message-id":"10","sms":{"originator":"1122","content":{"text":"This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, This is a longer message than 640 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, now,"}}}]}`,
			},
			{
				Body: `{"messages":[{"recipient":"99999999999","message-id":"10.2","sms":{"originator":"1122","content":{"text":"I need to keep adding more things to make it work"}}}]}`,
			},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+18686846481",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/broker-api/send": {
				httpx.NewMockResponse(200, nil, []byte(validMessage)),
			},
		},
	},
	{
		Label:   "Error sending",
		MsgText: "Error Sending",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/broker-api/send": {
				httpx.NewMockResponse(400, nil, []byte(`not json`)),
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PM", "1122", "UZ",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":  "Password",
			"username":  "Username",
			"shortcode": "1122",
			"base_url":  "http://example.com",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("Username", "Password")}, nil)
}
