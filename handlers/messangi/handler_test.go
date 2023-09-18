package messangi

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM", nil),
}

const (
	receiveURL = "/c/mg/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "mo=Msg&mobile=18765422035",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+18765422035"},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Data:                 "mo=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'mobile'"},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+18765422035",
		MockResponseBody:   `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+18765422035",
		MockResponseBody:   `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+18765422035",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `<response><input>sendMT</input><status>OK</status><description>Completed</description></response>`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid Parameters",
		MsgText:            "Invalid Parameters",
		MsgURN:             "tel:+18765422035",
		MockResponseBody:   "",
		MockResponseStatus: 404,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Response",
		MsgURN:             "tel:+18765422035",
		MockResponseBody:   `<response><input>sendMT</input><status>ERROR</status><description>Completed</description></response>`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueUnexpected("status", "OK")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MG", "2020", "JM",
		map[string]any{
			"public_key":  "my-public-key",
			"private_key": "my-private-key",
			"instance_id": 7,
			"carrier_id":  2,
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"my-private-key"}, nil)
}
