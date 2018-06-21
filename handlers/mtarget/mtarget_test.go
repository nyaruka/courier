package mtarget

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
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
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+923161909799")},
	{Label: "Invalid URN", URL: receiveURL, Data: receiveInvalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive Stop", URL: receiveURL, Data: receiveStop, Status: 200, Response: "Accepted",
		URN: Sp("tel:+923161909799"), ChannelEvent: Sp("stop_contact")},
	{Label: "Receive Missing From", URL: receiveURL, Data: receiveMissingFrom, Status: 400, Response: "missing required field 'Msisdn'"},

	{Label: "Receive Part 2", URL: receiveURL, Data: receivePart2, Status: 200, Response: "received"},
	{Label: "Receive Part 1", URL: receiveURL, Data: receivePart1, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+923161909799")},

	{Label: "Status Delivered", URL: statusURL, Data: statusDelivered, Status: 200, Response: "Accepted",
		ExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), MsgStatus: Sp("D")},
	{Label: "Status Failed", URL: statusURL, Data: statusFailed, Status: 200, Response: "Accepted",
		ExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), MsgStatus: Sp("F")},
	{Label: "Status Missing ID", URL: statusURL, Data: statusMissingID, Status: 400, Response: "missing required field 'MsgId'"},
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
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `{"results":[{"code": "0", "ticket": "externalID"}]}`, ResponseStatus: 200,
		URLParams: map[string]string{
			"msisdn":       "+250788383383",
			"msg":          "Simple Message",
			"username":     "Username",
			"password":     "Password",
			"serviceid":    "2020",
			"allowunicode": "true",
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `{"results":[{"code": "0", "ticket": "externalID"}]}`, ResponseStatus: 200,
		URLParams: map[string]string{"msg": "☺"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{"results":[{"code": "0", "ticket": "externalID"}]}`, ResponseStatus: 200,
		URLParams: map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{"results":[{"code": "3", "ticket": "null"}]}`, ResponseStatus: 403,
		SendPrep: setSendURL},
	{Label: "Error Response",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: `{"results":[{"code": "3", "ticket": "null"}]}`, ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
