package m3tech

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveValidMessage = "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?from=+923161909799&text=hello+world"
	receiveInvalidURN   = "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?from=MTN&text=hello+world"
	receiveMissingFrom  = "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?text=hello"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: " ", Status: 200, Response: "SMS Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+923161909799")},
	{Label: "Invalid URN", URL: receiveInvalidURN, Data: " ", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No From", URL: receiveMissingFrom, Data: " ", Status: 400, Response: "missing required field 'from'"},
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
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		URLParams: map[string]string{
			"MobileNo":    "250788383383",
			"SMS":         "Simple Message",
			"SMSChannel":  "0",
			"AuthKey":     "m3-Tech",
			"HandsetPort": "0",
			"MsgHeader":   "2020",
			"Telco":       "0",
			"SMSType":     "0",
			"UserId":      "Username",
			"Password":    "Password",
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		URLParams: map[string]string{"SMS": "☺", "SMSType": "7"},
		SendPrep:  setSendURL},
	{Label: "Smart Encoding",
		Text: "Fancy “Smart” Quotes", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		URLParams: map[string]string{"SMS": `Fancy "Smart" Quotes`, "SMSType": "0"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `[{"Response": "0"}]`, ResponseStatus: 200,
		URLParams: map[string]string{"SMS": "My pic!\nhttps://foo.bar/image.jpg", "SMSType": "0"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `[{"Response": "101"}]`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
