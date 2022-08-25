package mtarget

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var (
	receiveURL = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = "Msisdn=+923161909799&Content=hello+world&Keyword=Default"
	receiveInvalidURN   = "Msisdn=MTN&Content=hello+world&Keyword=Default"
	receiveStop         = "Msisdn=+923161909799&Content=Stop&Keyword=Stop"
	receiveMissingFrom  = "Content=hello&Keyword=Default"

	receivePart2 = "Msisdn=+923161909799&Content=world&Keyword=Default&msglong.id=longmsg&msglong.msgcount=2&msglong.msgref=2"
	receivePart1 = "Msisdn=+923161909799&Content=hello+&Keyword=Default&msglong.id=longmsg&msglong.msgcount=2&msglong.msgref=1"

	statusURL = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"

	statusDelivered = "MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=3"
	statusFailed    = "MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=4"
	statusMissingID = "status?Status=4"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: Sp("tel:+923161909799")},
	{Label: "Invalid URN", URL: receiveURL, Data: receiveInvalidURN, ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Receive Stop", URL: receiveURL, Data: receiveStop, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedURN: Sp("tel:+923161909799"), ExpectedChannelEvent: courier.StopContact},
	{Label: "Receive Missing From", URL: receiveURL, Data: receiveMissingFrom, ExpectedStatus: 400, ExpectedResponse: "missing required field 'Msisdn'"},

	{Label: "Receive Part 2", URL: receiveURL, Data: receivePart2, ExpectedStatus: 200, ExpectedResponse: "received"},
	{Label: "Receive Part 1", URL: receiveURL, Data: receivePart1, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("hello world"), ExpectedURN: Sp("tel:+923161909799")},

	{Label: "Status Delivered", URL: statusURL, Data: statusDelivered, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), ExpectedMsgStatus: Sp("D")},
	{Label: "Status Failed", URL: statusURL, Data: statusFailed, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), ExpectedMsgStatus: Sp("F")},
	{Label: "Status Missing ID", URL: statusURL, Data: statusMissingID, ExpectedStatus: 400, ExpectedResponse: "missing required field 'MsgId'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"results":[{"code": "0", "ticket": "externalID"}]}`,
		MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"msisdn":       "+250788383383",
			"msg":          "Simple Message",
			"username":     "Username",
			"password":     "Password",
			"serviceid":    "2020",
			"allowunicode": "true",
		},
		ExpectedStatus: "W",
		SendPrep:       setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"results":[{"code": "0", "ticket": "externalID"}]}`,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"msg": "☺"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"results":[{"code": "0", "ticket": "externalID"}]}`,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg"},
		ExpectedStatus:     "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"results":[{"code": "3", "ticket": "null"}]}`,
		MockResponseStatus: 403,
		ExpectedStatus:     "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"results":[{"code": "3", "ticket": "null"}]}`,
		MockResponseStatus: 200,
		ExpectedStatus:     "F",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("Error status code, failing permanently", "")},
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
