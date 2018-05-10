package dart

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var daTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID", nil),
}

var (
	receiveURL = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	validMessage   = receiveURL + "?userid=testusr&password=test&original=6289881134560&sendto=2020&message=Msg"
	invalidMessage = receiveURL
	externalURN    = receiveURL + "?userid=testusr&password=test&original=cmp-oodddqddwdwdcd&sendto=2020&message=Msg"

	validStatus        = statusURL + "?status=10&messageid=12345"
	validPartStatus    = statusURL + "?status=10&messageid=12345.2"
	failedStatus       = statusURL + "?status=30&messageid=12345"
	badStatus          = statusURL + "?status=foo&messageid=12345"
	badStatusMessageID = statusURL + "?status=10&messageid=abc"
	missingStatus      = statusURL + "?messageid=12345"
)

var daTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validMessage, Status: 200, Response: "000", Text: Sp("Msg"), URN: Sp("tel:+6289881134560")},
	{Label: "Receive Valid", URL: externalURN, Status: 200, Response: "000", Text: Sp("Msg"), URN: Sp("ext:cmp-oodddqddwdwdcd")},
	{Label: "Receive Invalid", URL: invalidMessage, Status: 400, Response: "missing required parameters original and sendto"},

	{Label: "Valid Status", URL: validStatus, Status: 200, Response: "000", MsgStatus: Sp("D")},
	{Label: "Valid Status", URL: validPartStatus, Status: 200, Response: "000", MsgStatus: Sp("D")},
	{Label: "Failed Status", URL: failedStatus, Status: 200, Response: "000", MsgStatus: Sp("F")},
	{Label: "Missing Status", URL: missingStatus, Status: 400, Response: "parameters messageid and status should not be empty"},
	{Label: "Missing Status", URL: badStatus, Status: 400, Response: "parsing failed: status 'foo' is not an integer"},
	{Label: "Missing Status", URL: badStatusMessageID, Status: 400, Response: "parsing failed: messageid 'abc' is not an integer"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	daHandler := h.(*handler)
	daHandler.sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:         "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:          "tel:+250788383383",
		Status:       "W",
		URLParams:    map[string]string{"message": "I need to keep adding more things to make it work", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10.2"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		URLParams:    map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"message": "Error Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Authentication Error",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "001", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Account Expired",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "101", ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultDAChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
		})

	RunChannelSendTestCases(t, defaultDAChannel, NewHandler("DA", "Dartmedia", sendURL, maxMsgLength), defaultSendTestCases, nil)
}
