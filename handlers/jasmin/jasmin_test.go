package jasmin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL          = "/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveValidMessage = "content=%05v%05nement&coding=0&From=2349067554729&To=2349067554711&id=1001"
	receiveMissingTo    = "content=%05v%05nement&coding=0&From=2349067554729&id=1001"
	invalidURN          = "content=%05v%05nement&coding=0&From=MTN&To=2349067554711&id=1001"

	statusURL       = "/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	statusDelivered = "id=external1&dlvrd=1"
	statusFailed    = "id=external1&err=1"
	statusUnknown   = "id=external1&err=0&dlvrd=0"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JS", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "ACK/Jasmin",
		Text: Sp("événement"), URN: Sp("tel:+2349067554729"), ExternalID: Sp("1001")},
	{Label: "Receive Missing To", URL: receiveURL, Data: receiveMissingTo, Status: 400,
		Response: "field 'to' required"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400,
		Response: "phone number supplied is not a number"},
	{Label: "Status Delivered", URL: statusURL, Data: statusDelivered, Status: 200, Response: "ACK/Jasmin",
		MsgStatus: Sp("D"), ExternalID: Sp("external1")},
	{Label: "Status Failed", URL: statusURL, Data: statusFailed, Status: 200, Response: "ACK/Jasmin",
		MsgStatus: Sp("F"), ExternalID: Sp("external1")},
	{Label: "Status Missing", URL: statusURL, Status: 400, Data: "nothing",
		Response: "field 'id' required"},
	{Label: "Status Unknown", URL: statusURL, Status: 400, Data: statusUnknown,
		Response: "must have either dlvrd or err set to 1"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("send_url", s.URL)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `Success "External ID1"`, ResponseStatus: 200, ExternalID: "External ID1",
		URLParams: map[string]string{"content": "Simple Message", "to": "250788383383", "coding": "0",
			"dlr-level": "2", "dlr": "yes", "dlr-method": http.MethodPost, "username": "Username", "password": "Password",
			"dlr-url": "https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text:         "☺",
		Status:       "W",
		ResponseBody: `Success "External ID1"`, ResponseStatus: 200,
		URLParams: map[string]string{"content": "?"},
		SendPrep:  setSendURL},
	{Label: "Smart Encoding",
		Text: "Fancy “Smart” Quotes", URN: "tel:+250788383383", HighPriority: false,
		Status:       "W",
		ResponseBody: `Success "External ID1"`, ResponseStatus: 200,
		URLParams: map[string]string{"content": `Fancy "Smart" Quotes`},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", HighPriority: true, Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `Success "External ID1"`, ResponseStatus: 200,
		URLParams: map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383", HighPriority: false,
		Status:       "E",
		ResponseBody: "Failed Sending", ResponseStatus: 401,
		URLParams: map[string]string{"content": `Error Message`},
		SendPrep:  setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JS", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username"})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
