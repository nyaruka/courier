package plivo

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "MY", nil),
}

var (
	receiveURL = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive   = "To=2020&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	invalidURN     = "To=2020&From=MTN&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	invalidAddress = "To=1515&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	missingParams  = "From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"

	validStatus          = "MessageUUID=12345&status=delivered&To=%2B60124361111&From=2020"
	validSentStatus      = "ParentMessageUUID=12345&status=sent&MessageUUID=123&To=%2B60124361111&From=2020"
	invalidStatusAddress = "ParentMessageUUID=12345&status=sent&MessageUUID=123&To=%2B60124361111&From=1515"
	unknownStatus        = "MessageUUID=12345&status=UNKNOWN&To=%2B60124361111&From=2020"
)

var testCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText: Sp("Hello"), ExpectedURN: "tel:+60124361111", ExpectedExternalID: "abc1234"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, ExpectedRespStatus: 400, ExpectedBodyContains: "phone number supplied is not a number"},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid to number [1515], expecting [2020]"},
	{Label: "Missing Params", URL: receiveURL, Data: missingParams, ExpectedRespStatus: 400, ExpectedBodyContains: "Field validation for 'To' failed"},

	{
		Label:                "Valid Status",
		URL:                  statusURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12345", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Sent Status",
		URL:                  statusURL,
		Data:                 validSentStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12345", Status: courier.MsgStatusSent},
		},
	},
	{Label: "Invalid Status Address", URL: statusURL, Data: invalidStatusAddress, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid to number [1515], expecting [2020]"},
	{Label: "Unkown Status", URL: statusURL, Data: unknownStatus, ExpectedRespStatus: 200, ExpectedBodyContains: `ignoring unknown status 'UNKNOWN'`},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL + "/%s/"
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "message_uuid":["abc123"] }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		ExpectedRequestBody: `{"src":"2020","dst":"250788383383","text":"Simple Message ☺","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:            setSendURL},
	{Label: "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "message_uuid":["abc123"] }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		ExpectedRequestBody: `{"src":"2020","dst":"250788383383","text":"I need to keep adding more things to make it work","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:            setSendURL},
	{Label: "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "message_uuid":["abc123"] }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		ExpectedRequestBody: `{"src":"2020","dst":"250788383383","text":"My pic!\nhttps://foo.bar/image.jpg","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:            setSendURL},
	{Label: "No External Id",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "E",
		MockResponseBody:   `{ "missing":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 200,
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "unable to parse response body from Plivo")},
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		ExpectedRequestBody: `{"src":"2020","dst":"250788383383","text":"No External ID","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:            setSendURL},
	{Label: "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		ExpectedMsgStatus:   "E",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"src":"2020","dst":"250788383383","text":"Error Message","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:            setSendURL},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "US",
		map[string]any{
			configPlivoAuthID:    "AuthID",
			configPlivoAuthToken: "AuthToken",
			configPlivoAPPID:     "AppID",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("AuthID", "AuthToken")}, nil)
}
