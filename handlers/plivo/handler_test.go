package plivo

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "MY", []string{urns.Phone.Prefix}, nil),
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
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, ExpectedRespStatus: 400, ExpectedBodyContains: "not a possible number"},
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

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.plivo.com/v1/Account/AuthID/Message/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "message_uuid":["abc123"] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
			},
			Body: `{"src":"2020","dst":"250788383383","text":"Simple Message ☺","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		}},
		ExpectedExtIDs: []string{"abc123"},
	},
	{Label: "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.plivo.com/v1/Account/AuthID/Message/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "message_uuid":["abc123"] }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "message_uuid":["abc123"] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
				},
				Body: `{"src":"2020","dst":"250788383383","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
			},
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
				},
				Body: `{"src":"2020","dst":"250788383383","text":"I need to keep adding more things to make it work","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
			},
		},
		ExpectedExtIDs: []string{"abc123", "abc123"},
	},
	{Label: "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.plivo.com/v1/Account/AuthID/Message/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "message_uuid":["abc123"] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
			},
			Body: `{"src":"2020","dst":"250788383383","text":"My pic!\nhttps://foo.bar/image.jpg","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		}},
		ExpectedExtIDs: []string{"abc123"},
	},
	{Label: "No External Id",
		MsgText: "No External ID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.plivo.com/v1/Account/AuthID/Message/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "missing":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
			},
			Body: `{"src":"2020","dst":"250788383383","text":"No External ID","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		}},
		ExpectedError: courier.ErrResponseContent,
	},
	{Label: "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.plivo.com/v1/Account/AuthID/Message/": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"src":"2020","dst":"250788383383","text":"Error Message","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configPlivoAuthID:    "AuthID",
			configPlivoAuthToken: "AuthToken",
			configPlivoAPPID:     "AppID",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("AuthID", "AuthToken")}, nil)
}
