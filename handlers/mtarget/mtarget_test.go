package mtarget

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveValidMessage = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Msisdn=+923161909799&Content=hello+world&Keyword=Default"
	receiveStop         = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Msisdn=+923161909799&Content=Stop&Keyword=Stop"
	receiveMissingFrom  = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?Content=hello&Keyword=Default"

	statusDelivered = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=3"
	statusFailed    = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=4"
	statusMissingID = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?Status=4"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: " ", Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+923161909799")},
	{Label: "Receive Stop", URL: receiveStop, Data: " ", Status: 200, Response: "Accepted",
		URN: Sp("tel:+923161909799"), ChannelEvent: Sp("stop_contact")},
	{Label: "Receive Missing From", URL: receiveMissingFrom, Data: " ", Status: 400, Response: "missing required field 'Msisdn'"},

	{Label: "Status Delivered", URL: statusDelivered, Data: " ", Status: 200, Response: "Accepted",
		ExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), MsgStatus: Sp("D")},
	{Label: "Status Failed", URL: statusFailed, Data: " ", Status: 200, Response: "Accepted",
		ExternalID: Sp("12a7ee90-50ce-11e7-80ae-00000a0a643c"), MsgStatus: Sp("F")},
	{Label: "Status Missing ID", URL: statusMissingID, Data: " ", Status: 400, Response: "missing required field 'MsgId'"},
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
			"msisdn":   "+250788383383",
			"msg":      "Simple Message",
			"username": "Username",
			"password": "Password",
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

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases)
}
