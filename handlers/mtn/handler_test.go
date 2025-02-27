package mtn

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

var (
	receiveURL = "/c/mtn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var helloMsg = `{
	"id":null,
	"senderAddress":"242064661201",
	"receiverAddress":"2020",
	"message":"Hello there",
	"created":1678794364855,
	"submittedDate":null
}
`

var invalidURN = `{
	"id":null,
	"senderAddress":"foobar",
	"receiverAddress":"2020",
	"message":"Hello there",
	"created":1678794364855,
	"submittedDate":null
}
`

var validStatus = `{
	"TransactionID": "rrt-58503",
	"clientCorrelator": "string",
	"deliveryStatus":  "DeliveredToTerminal"
}
`

var validDeliveredStatus = `{
	"TransactionID": "rrt-58503",
	"clientCorrelator": "string",
	"deliveryStatus": "DELIVRD"
}
`

var ignoredStatus = `{
	"TransactionID": "rrt-58503",
	"clientCorrelator": "string",
	"deliveryStatus": "MessageWaiting"
}
`

var expiredStatus = `{
	"TransactionID": "rrt-58503",
	"clientCorrelator": "string",
	"deliveryStatus": "EXPIRED"
}
`

var uknownStatus = `{
	"TransactionID": "rrt-58503",
	"clientCorrelator": "string",
	"deliveryStatus": "blabla"
}
`
var missingTransactionID = `{
	"TransactionID": null,
	"clientCorrelator": "string",
	"deliveryStatus": "EXPIRED"
}`

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello there"),
		ExpectedURN:          "tel:+242064661201",
		ExpectedDate:         time.Date(2023, time.March, 14, 11, 46, 4, 855000000, time.UTC),
	},
	{
		Label:                "Receive invalid URN Message",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive Valid Status",
		URL:                  receiveURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "rrt-58503", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Receive Valid delivered Status",
		URL:                  receiveURL,
		Data:                 validDeliveredStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "rrt-58503", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Receive ignored Status",
		URL:                  receiveURL,
		Data:                 ignoredStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `Ignored`,
	},
	{
		Label:                "Receive ignored Status, missing transaction ID",
		URL:                  receiveURL,
		Data:                 missingTransactionID,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `Ignored`,
	},
	{
		Label:                "Receive expired Status",
		URL:                  receiveURL,
		Data:                 expiredStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "rrt-58503", Status: courier.MsgStatusFailed},
		},
	},
	{
		Label:                "Receive uknown Status",
		URL:                  receiveURL,
		Data:                 uknownStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"}),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mtn.com/v2/messages/sms/outbound": {
				httpx.NewMockResponse(201, nil, []byte(`{ "transactionId":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Simple Message ☺","clientCorrelator":"10"}`,
		}},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mtn.com/v2/messages/sms/outbound": {
				httpx.NewMockResponse(201, nil, []byte(`{ "transactionId":"OzYDlvf3SQVc" }`)),
			},
		},

		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"My pic!\nhttps://foo.bar/image.jpg","clientCorrelator":"10"}`,
		}},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc"},
	},
	{
		Label:   "No External Id",
		MsgText: "No External ID",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mtn.com/v2/messages/sms/outbound": {
				httpx.NewMockResponse(200, nil, []byte(`{"statusCode":"0000"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"No External ID","clientCorrelator":"10"}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("transactionId")},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mtn.com/v2/messages/sms/outbound": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Error Message","clientCorrelator":"10"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

var cpAddressOutgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.mtn.com/v2/messages/sms/outbound": {
				httpx.NewMockResponse(201, nil, []byte(`{ "transactionId":"OzYDlvf3SQVc" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer ACCESS_TOKEN",
			},
			Body: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Simple Message ☺","clientCorrelator":"10","cpAddress":"FOO"}`,
		}},
		ExpectedExtIDs: []string{"OzYDlvf3SQVc"},
	},
}

func setupBackend(mb *test.MockBackend) {
	// ensure there's a cached access token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), outgoingCases, []string{"customer-key", "customer-secret123"}, setupBackend)

	var cpAddressChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key", configCPAddress: "FOO"})
	RunOutgoingTestCases(t, cpAddressChannel, newHandler(), cpAddressOutgoingCases, []string{"customer-key", "customer-secret123"}, setupBackend)
}
