package clickatell

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSendURL takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "Unicode ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "Unicode ☺", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "1", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:   "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:    "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "I need to keep adding more things to make it work", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "ID: id1002", ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Error Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Invalid Token",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_id": "API-ID", "user": "Username", "password": "Password", "unicode": "0", "mo": "1", "callback": "7", "concat": "3"},
		ResponseBody: "Invalid API token", ResponseStatus: 401,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
			courier.ConfigAPIID:    "API-ID",
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
			courier.ConfigAPIID:    "12345",
		}),
}

var (
	statusURL  = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&text=Hello+World"

	receiveValidMessageISO8859_1_1 = `/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&text=%05%EF%BF%BD%EF%BF%BD%034%02%41i+mapfumbamwe+vana+4+kuwacha+handingapedze+izvozvo+ndozvikukonzera+kt+varoorwe+varipwere+ngapaonekwe+ipapo+ndatenda.&charset=ISO-8859-1`

	receiveValidMessageISO8859_1_2 = `/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&text=Artwell+S%ECbbnda&charset=ISO-8859-1`

	receiveValidMessageISO8859_1_3 = `/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&text=a%3F+%A3irvine+stinta%3F%A5.++&charset=ISO-8859-1`

	receiveValidMessageISO8859_1_4 = `/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&text=when%3F+or+What%3F+is+this+&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&charset=ISO-8859-1`

	receiveValidMessageUTF16BE = `/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=%2B250788123123&moMsgId=id1234&from=250788383383&timestamp=2012-10-10+10%3A10%3A10&text=%00m%00e%00x%00i%00c%00o%00+%00k%00+%00m%00i%00s%00+%00p%00a%00p%00a%00s%00+%00n%00o%00+%00t%00e%00n%00%ED%00a%00+%00d%00i%00n%00e%00r%00o%00+%00p%00a%00r%00a%00+%00c%00o%00m%00p%00r%00a%00r%00n%00o%00s%00+%00l%00o%00+%00q%00+%00q%00u%00e%00r%00%ED%00a%00m%00o%00s%00.%00.&charset=UTF-16BE`

	receiveInvalidAPIID = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?api_id=123"

	statusFailed    = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?apiMsgId=id1234&status=001"
	statusDelivered = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?apiMsgId=id1234&status=004"

	statusDeliveredValidAPIID   = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?api_id=12345&apiMsgId=id1234&status=004"
	statusDeliveredInvalidAPIID = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?api_id=123&apiMsgId=id1234&status=004"
	statusUnexpected            = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?apiMsgId=id1234&status=020"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Valid Receive", URL: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+250788383383"), ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},
	{Label: "Ignored missing parameters", URL: receiveURL, Status: 200, Response: `missing one of 'from', 'text', 'moMsgId' or 'timestamp' in request parameters.`},
	{Label: "Valid Receive ISO-8859-1 (1)", URL: receiveValidMessageISO8859_1_1, Status: 200, Response: "Accepted",
		Text: Sp(`ï¿½ï¿½4Ai mapfumbamwe vana 4 kuwacha handingapedze izvozvo ndozvikukonzera kt varoorwe varipwere ngapaonekwe ipapo ndatenda.`),
		URN:  Sp("tel:+250788383383"), ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},

	{Label: "Valid Receive ISO-8859-1 (2)", URL: receiveValidMessageISO8859_1_2, Status: 200, Response: "Accepted",
		Text: Sp(`Artwell Sìbbnda`), URN: Sp("tel:+250788383383"),
		ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},

	{Label: "Valid Receive ISO-8859-1 (3)", URL: receiveValidMessageISO8859_1_3, Status: 200, Response: "Accepted",
		Text: Sp(`a? £irvine stinta?¥.  `), URN: Sp("tel:+250788383383"),
		ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},

	{Label: "Valid Receive ISO-8859-1 (4)", URL: receiveValidMessageISO8859_1_4, Status: 200, Response: "Accepted",
		Text: Sp(`when? or What? is this `), URN: Sp("tel:+250788383383"),
		ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},

	{Label: "Valid Receive UTF-16BE", URL: receiveValidMessageUTF16BE, Status: 200, Response: "Accepted",
		Text: Sp("mexico k mis papas no tenýa dinero para comprarnos lo q querýamos.."), URN: Sp("tel:+250788383383"),
		ExternalID: Sp("id1234"), Date: Tp(time.Date(2012, 10, 10, 8, 10, 10, 0, time.UTC))},

	{Label: "Receive with Invalid API ID", URL: receiveInvalidAPIID, Status: 400, Response: `invalid API ID for message delivery: 123`},
	{Label: "Ignored status report", URL: statusURL, Status: 200, Response: `missing one of 'apiMsgId' or 'status' in request parameters.`},
	{Label: "Valid Failed status report", URL: statusFailed, Status: 200, Response: `"status":"F"`},
	{Label: "Valid Delivered status report", URL: statusDelivered, Status: 200, Response: `"status":"D"`},
	{Label: "Valid Delivered status report with API ID", URL: statusDeliveredValidAPIID, Status: 200, Response: `"status":"D"`},
	{Label: "Valid Delivered status report with Invalid API ID", URL: statusDeliveredInvalidAPIID, Status: 400, Response: `invalid API ID for status report: 123`},
	{Label: "Unexpected status report", URL: statusUnexpected, Status: 400, Response: `unknown status '020', must be one of 001, 002, 003, 004, 005, 006, 007, 008, 009, 010, 011, 012, 014`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
