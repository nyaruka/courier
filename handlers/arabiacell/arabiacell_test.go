package arabiacell

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US", nil),
}

const (
	receiveURL = "/c/ac/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []ChannelHandleTestCase{
	{
		Label:              "Receive Valid",
		URL:                receiveURL,
		Data:               "B=Msg&M=254791541111",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "Message Accepted",
		ExpectedMsgText:    Sp("Msg"),
		ExpectedURN:        "tel:+254791541111",
	},
	{
		Label:              "Receive Missing Number",
		URL:                receiveURL,
		Data:               "B=Msg",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "required field 'M'",
	},
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
	{
		Label:          "Plain Send",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody: `<response>
		<code>204</code>
		<text>MT is successfully sent</text>
		<message_id>external1</message_id>
</response>`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"userName":      "user1",
			"password":      "pass1",
			"handlerType":   "send_msg",
			"serviceId":     "service1",
			"msisdn":        "+250788383383",
			"messageBody":   "Simple Message ☺\nhttps://foo.bar/image.jpg",
			"chargingLevel": "0",
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "external1",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid XML",
		MsgText:            "Invalid XML",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `not xml`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("EOF", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Response",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `<response><code>501</code><text>failure</text><message_id></message_id></response>`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("Received invalid response code: 501", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Bad Gateway`,
		MockResponseStatus: 501,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configServiceID:        "service1",
			configChargingLevel:    "0",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
