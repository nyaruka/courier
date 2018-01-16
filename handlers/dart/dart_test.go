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

var h9TestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "H9", "2020", "ID", nil),
}

var (
	daReceiveURL = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	daStatusURL  = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	h9ReceiveURL = "/c/h9/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	h9StatusURL  = "/c/h9/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	validMessage = "?userid=testusr&password=test&original=6289881134560&sendto=2020&message=Msg"
	validStatus = "?status=10&messageid=12345"
	badStatus = "?status=foo&messageid=12345"
	badStatusMessageID = "?status=10&messageid=abc"
	missingStatus = "?messageid=12345"

	validDAReceive = daReceiveURL + validMessage
	validDAStatus  = daStatusURL + validStatus
	missingDAStatus = daStatusURL + missingStatus
	badDAStatus = daStatusURL + badStatus
	badDAStatusMessageID = daStatusURL + badStatusMessageID

	validH9Receive = h9ReceiveURL + validMessage
	validH9Status = h9StatusURL + validStatus
	missingH9Status = h9StatusURL + missingStatus
	badH9Status = h9StatusURL + badStatus
	badH9StatusMessageID = h9StatusURL + badStatusMessageID

)

var daTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validDAReceive, Status: 200, Response: "000",
		Text: Sp("Msg"), URN: Sp("tel:+6289881134560")},
	{Label: "Valid Status", URL: validDAStatus, Status: 200, Response: "000"},
	{Label: "Missing Status", URL: missingDAStatus, Status: 400, Response: "parameters messageid and status should not be null"},
	{Label: "Missing Status", URL: badDAStatus, Status: 400, Response: "parsing failed: status 'foo' is not an integer"},
	{Label: "Missing Status", URL: badDAStatusMessageID, Status: 400, Response: "parsing failed: messageid 'abc' is not an integer"},
}

var h9TestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validH9Receive, Status: 200, Response: "000",
		Text: Sp("Msg"), URN: Sp("tel:+6289881134560")},
	{Label: "Valid Status", URL: validH9Status, Status: 200, Response: "000"},
	{Label: "Missing Status", URL: missingH9Status, Status: 400, Response: "parameters messageid and status should not be null"},
	{Label: "Missing Status", URL: badH9Status, Status: 400, Response: "parsing failed: status 'foo' is not an integer"},
	{Label: "Missing Status", URL: badH9StatusMessageID, Status: 400, Response: "parsing failed: messageid 'abc' is not an integer"},

}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, daTestChannels, NewHandler("DA", "DartMedia"), daTestCases)
	RunChannelTestCases(t, h9TestChannels, NewHandler("H9", "Hub9"), h9TestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, daTestChannels, NewHandler("DA", "DartMedia"), daTestCases)
	RunChannelBenchmarks(b, h9TestChannels, NewHandler("H9", "Hub9"), h9TestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	dartmediaSendURL = server.URL
	hub9SendURL = server.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W",
		URLParams:    map[string]string{"message": "I need to keep adding more things to make it work", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W",
		URLParams:    map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "000", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:   map[string]string{"message": "Error Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Authentication Error",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "E",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "001", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Account Expired",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "E",
		URLParams:    map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ResponseBody: "101", ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	dartmediaMaxMsgLength = 160
	hub9MaxMsgLength = 160
	var defaultDAChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
			courier.ConfigAPIID:    "API-ID",
		})

	var defaultH9Channel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "H9", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
			courier.ConfigAPIID:    "API-ID",
		})

	RunChannelSendTestCases(t, defaultDAChannel, NewHandler("DA", "Dartmedia"), defaultSendTestCases)
	RunChannelSendTestCases(t, defaultH9Channel, NewHandler("H9", "Hub9"), defaultSendTestCases)
}
