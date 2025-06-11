package mblox

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MB", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"username": "zv-username", "password": "zv-password"}),
}

var (
	receiveURL = "/c/mb/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = `{
		"id": "OzQ5UqIOdoY8",
		"from": "12067799294",
		"to": "18444651185",
		"body": "Hello World",
		"type": "mo_text",
		"received_at": "2016-03-30T19:33:06.643Z"
	}`

	invalidURN = `{
		"id": "OzQ5UqIOdoY8",
		"from": "MTN",
		"to": "18444651185",
		"body": "Hello World",
		"type": "mo_text",
		"received_at": "2016-03-30T19:33:06.643Z"
	}`

	missingParamsRecieve = `{
		"id": "OzQ5UqIOdoY8",
		"to": "18444651185",
		"body": "Hello World",
		"type": "mo_text",
		"received_at": "2016-03-30T19:33:06.643Z"
	}`

	validStatus = `{
		"batch_id": "12345",
		"status": "Delivered",
		"type": "recipient_delivery_report_sms"
	}`

	unknownStatus = `{
		"batch_id": "12345",
		"status": "INVALID",
		"type": "recipient_delivery_report_sms"
	}`

	missingBatchID = `{
		"status": "Delivered",
		"type": "recipient_delivery_report_sms"
	}`
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "tel:+12067799294",
		ExpectedDate:         time.Date(2016, 3, 30, 19, 33, 06, 643000000, time.UTC),
		ExpectedExternalID:   "OzQ5UqIOdoY8",
	},
	{
		Label:                "Receive Missing Params",
		URL:                  receiveURL,
		Data:                 missingParamsRecieve,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Status Valid",
		URL:                  receiveURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Status Unknown",
		URL:                  receiveURL,
		Data:                 unknownStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `unknown status 'INVALID'`,
	},
	{
		Label:                "Status Missing Batch ID",
		URL:                  receiveURL,
		Data:                 missingBatchID,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing one of 'batch_id' or 'status' in request body",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mblox.com/xms/v1/Username/batches": {
				httpx.NewMockResponse(200, nil, []byte(`{ "id":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer Password",
			},
			Body: `{"from":"2020","to":["250788383383"],"body":"Simple Message ☺","delivery_report":"per_recipient"}`,
		}},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc"},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mblox.com/xms/v1/Username/batches": {
				httpx.NewMockResponse(200, nil, []byte(`{ "id":"OzYDlvf3SQVc" }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "id":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Bearer Password",
				},
				Body: `{"from":"2020","to":["250788383383"],"body":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,","delivery_report":"per_recipient"}`,
			},
			{
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Accept":        "application/json",
					"Authorization": "Bearer Password",
				},
				Body: `{"from":"2020","to":["250788383383"],"body":"I need to keep adding more things to make it work","delivery_report":"per_recipient"}`,
			},
		},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc", "OzYDlvf3SQVc"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mblox.com/xms/v1/Username/batches": {
				httpx.NewMockResponse(200, nil, []byte(`{ "id":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer Password",
			},
			Body: `{"from":"2020","to":["250788383383"],"body":"My pic!\nhttps://foo.bar/image.jpg","delivery_report":"per_recipient"}`,
		}},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc"},
	},
	{
		Label:   "No External Id",
		MsgText: "No External ID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mblox.com/xms/v1/Username/batches": {
				httpx.NewMockResponse(200, nil, []byte(`{ "missing":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer Password",
			},
			Body: `{"from":"2020","to":["250788383383"],"body":"No External ID","delivery_report":"per_recipient"}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("id")},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mblox.com/xms/v1/Username/batches": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"from":"2020","to":["250788383383"],"body":"Error Message","delivery_report":"per_recipient"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
