package clickatell

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var successSendResponse = `{"messages":[{"apiMessageId":"id1002","accepted":true,"to":"12067799299","error":null}],"error":null}`
var failSendResponse = `{"messages":[],"error":"Two-Way integration error - From number is not related to integration"}`

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"content": "Simple Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ResponseBody: successSendResponse, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "Unicode ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"content": "Unicode ☺", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ResponseBody: successSendResponse, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "id1002",
		URLParams:    map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ResponseBody: successSendResponse, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"content": "Error Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Error Response",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"content": "Error Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ResponseBody: failSendResponse, ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]interface{}{
			courier.ConfigAPIKey: "API-KEY",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]interface{}{
			courier.ConfigAPIKey: "12345",
		}),
}

var (
	statusURL  = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/ct/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = `{ 
		"messageId":"1234", 
		"fromNumber": "250788383383", 
		"timestamp":1516217711000, 
		"text": "Hello World!", 
		"charset":"UTF-8"
	}`

	invalidURN = `{
		"messageId":"1234",
		"fromNumber": "MTN",
		"timestamp":1516217711000,
		"text": "Hello World!",
		"charset":"UTF-8"
	}`

	receiveValidMessageISO8859_1 = `{ 
		"messageId":"1234", 
		"fromNumber": "250788383383", 
		"timestamp":1516217711000, 
		"text": "hello%21", 
		"charset":"ISO-8859-1"
	}`

	receiveValidMessageUTF16BE = `{ 
		"messageId":"1234", 
		"fromNumber": "250788383383", 
		"timestamp":1516217711000, 
		"text": "%00m%00e%00x%00i%00c%00o%00+%00k%00+%00m%00i%00s%00+%00p%00a%00p%00a%00s%00+%00n%00o%00+%00t%00e%00n%00%ED%00a%00+%00d%00i%00n%00e%00r%00o%00+%00p%00a%00r%00a%00+%00c%00o%00m%00p%00r%00a%00r%00n%00o%00s%00+%00l%00o%00+%00q%00+%00q%00u%00e%00r%00%ED%00a%00m%00o%00s%00.%00.",
		"charset": "UTF-16BE"
	}`

	statusFailed = `{
		"messageId": "msg1",
		"statusCode": 5
	}`
	statusSent = `{
		"messageId": "msg1",
		"statusCode": 4
	}`

	statusUnexpected = `{
		"messageId": "msg1",
		"statusCode": -1
	}`
)

var testCases = []ChannelHandleTestCase{
	{Label: "Valid Receive", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Hello World!"), URN: Sp("tel:+250788383383"), ExternalID: Sp("1234"), Date: Tp(time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC))},
	{Label: "Valid Receive ISO-8859-1", URL: receiveURL, Data: receiveValidMessageISO8859_1, Status: 200, Response: "Accepted",
		Text: Sp(`hello!`), URN: Sp("tel:+250788383383"), ExternalID: Sp("1234"), Date: Tp(time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC))},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Error invalid JSON", URL: receiveURL, Data: "foo", Status: 400, Response: `unable to parse request JSON`},
	{Label: "Error missing JSON", URL: receiveURL, Data: "{}", Status: 400, Response: `missing one of 'messageId`},
	{Label: "Valid Receive UTF-16BE", URL: receiveURL, Data: receiveValidMessageUTF16BE, Status: 200, Response: "Accepted",
		Text: Sp("mexico k mis papas no tenýa dinero para comprarnos lo q querýamos.."), URN: Sp("tel:+250788383383"),
		ExternalID: Sp("1234"), Date: Tp(time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC))},

	{Label: "Valid Failed status report", URL: statusURL, Data: statusFailed, Status: 200, Response: `"status":"F"`},
	{Label: "Valid Delivered status report", URL: statusURL, Data: statusSent, Status: 200, Response: `"status":"S"`},
	{Label: "Unexpected status report", URL: statusURL, Data: statusUnexpected, Status: 400, Response: `unknown status '-1', must be one`},

	{Label: "Invalid status report", URL: statusURL, Data: "{}", Status: 400, Response: `missing one of 'messageId'`},
	{Label: "Invalid JSON", URL: statusURL, Data: "foo", Status: 400, Response: `unable to parse request JSON`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
