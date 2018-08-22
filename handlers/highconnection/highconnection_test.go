package highconnection

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US", nil),
}

var (
	receiveURL = "/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive       = "FROM=+33610346460&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14%3A26%3A06"
	invalidURN         = "FROM=MTN&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14%3A26%3A06"
	invalidDateReceive = "FROM=+33610346460&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14:26"
	validStatus        = statusURL + "?ret_id=12345&status=6"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: validReceive, Status: 200, Response: "Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+33610346460"),
		Date: Tp(time.Date(2015, 04, 02, 14, 26, 06, 0, time.UTC))},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive Missing Params", URL: receiveURL, Data: " ", Status: 400, Response: "validation for 'From' failed"},
	{Label: "Receive Invalid Date", URL: receiveURL, Data: invalidDateReceive, Status: 400, Response: "cannot parse"},
	{Label: "Status Missing Params", URL: statusURL, Status: 400, Response: "validation for 'Status' failed"},
	{Label: "Status Delivered", URL: validStatus, Status: 200, Response: `"status":"D"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:   "Simple Message",
		URN:    "tel:+250788383383",
		Status: "W",
		URLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "Simple Message",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"userdata":   "textit",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ResponseStatus: 200,
		SendPrep:       setSendURL},
	{Label: "Unicode Send",
		Text:   "☺",
		URN:    "tel:+250788383383",
		Status: "W",
		URLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "☺",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"userdata":   "textit",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ResponseStatus: 200,
		SendPrep:       setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W",
		URLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "I need to keep adding more things to make it work",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"userdata":   "textit",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ResponseStatus: 200,
		SendPrep:       setSendURL},
	{Label: "Send Attachement",
		Text:        "My pic!",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		URN:         "tel:+250788383383",
		Status:      "W",
		URLParams: map[string]string{
			"accountid":  "Username",
			"password":   "Password",
			"text":       "My pic!\nhttps://foo.bar/image.jpg",
			"to":         "+250788383383",
			"ret_id":     "10",
			"datacoding": "8",
			"userdata":   "textit",
			"ret_url":    "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status",
			"ret_mo_url": "https://localhost/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive",
		},
		ResponseStatus: 200,
		SendPrep:       setSendURL},

	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:         "E",
		ResponseStatus: 403,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
