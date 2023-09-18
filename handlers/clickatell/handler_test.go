package clickatell

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var successSendResponse = `{"messages":[{"apiMessageId":"id1002","accepted":true,"to":"12067799299","error":null}],"error":null}`
var failSendResponse = `{"messages":[],"error":"Two-Way integration error - From number is not related to integration"}`

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   successSendResponse,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"content": "Simple Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "id1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "Unicode ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   successSendResponse,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"content": "Unicode ☺", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "id1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   successSendResponse,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"content": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "id1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Error`,
		MockResponseStatus: 400,
		ExpectedURLParams:  map[string]string{"content": "Error Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   failSendResponse,
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"content": "Error Message", "to": "250788383383", "from": "2020", "apiKey": "API-KEY"},
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("apiMessageId")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]any{
			courier.ConfigAPIKey: "API-KEY",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"API-KEY"}, nil)
}

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CT", "2020", "US",
		map[string]any{
			courier.ConfigAPIKey: "12345",
		}),
}

const (
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
)

var testCases = []IncomingTestCase{
	{
		Label:                "Valid Receive",
		URL:                  receiveURL,
		Data:                 receiveValidMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello World!"),
		ExpectedURN:          "tel:+250788383383",
		ExpectedExternalID:   "1234",
		ExpectedDate:         time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC),
	},
	{
		Label:                "Valid Receive ISO-8859-1",
		URL:                  receiveURL,
		Data:                 receiveValidMessageISO8859_1,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(`hello!`),
		ExpectedURN:          "tel:+250788383383",
		ExpectedExternalID:   "1234",
		ExpectedDate:         time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC),
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Error invalid JSON",
		URL:                  receiveURL,
		Data:                 "foo",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unable to parse request JSON`,
	},
	{
		Label:                "Error missing JSON",
		URL:                  receiveURL,
		Data:                 "{}",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `missing one of 'messageId`,
	},
	{
		Label:                "Valid Receive UTF-16BE",
		URL:                  receiveURL,
		Data:                 receiveValidMessageUTF16BE,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("mexico k mis papas no tenýa dinero para comprarnos lo q querýamos.."),
		ExpectedURN:          "tel:+250788383383",
		ExpectedExternalID:   "1234",
		ExpectedDate:         time.Date(2018, 1, 17, 19, 35, 11, 0, time.UTC),
	},
	{
		Label:                "Valid Failed status report",
		URL:                  statusURL,
		Data:                 `{"messageId": "msg1", "statusCode": 5}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "msg1", Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Valid Delivered status report",
		URL:                  statusURL,
		Data:                 `{"messageId": "msg1", "statusCode": 4}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "msg1", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Unexpected status report",
		URL:                  statusURL,
		Data:                 `{"messageId": "msg1", "statusCode": -1}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unknown status '-1', must be one`,
	},
	{
		Label:                "Invalid status report",
		URL:                  statusURL,
		Data:                 "{}",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `missing one of 'messageId'`,
	},
	{
		Label:                "Invalid JSON",
		URL:                  statusURL,
		Data:                 "foo",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unable to parse request JSON`,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
