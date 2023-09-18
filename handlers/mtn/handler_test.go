package mtn

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"}),
}

var (
	receiveURL = "/c/mtn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	statusURL  = "/c/mtn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
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

var testCases = []IncomingTestCase{
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
		ExpectedBodyContains: "phone number supplied is not a number",
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
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	apiHostURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "OzYDlvf3SQVc",
		MockResponseBody:   `{ "transactionId":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 201,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Simple Message ☺","clientCorrelator":"10"}`,
		SendPrep:            setSendURL},
	{Label: "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "OzYDlvf3SQVc",
		MockResponseBody:   `{ "transactionId":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"My pic!\nhttps://foo.bar/image.jpg","clientCorrelator":"10"}`,
		SendPrep:            setSendURL},
	{Label: "No External Id",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "E",
		MockResponseBody:   `{"statusCode":"0000"}`,
		MockResponseStatus: 200,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("transactionId")},
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"No External ID","clientCorrelator":"10"}`,
		SendPrep:            setSendURL},
	{Label: "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		ExpectedMsgStatus:   "E",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Error Message","clientCorrelator":"10"}`,
		SendPrep:            setSendURL},
}

func setupBackend(mb *test.MockBackend) {
	// ensure there's a cached access token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

var cpAddressSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "OzYDlvf3SQVc",
		MockResponseBody:   `{ "transactionId":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 201,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Simple Message ☺","clientCorrelator":"10","cpAddress":"FOO"}`,
		SendPrep:            setSendURL},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"customer-key", "customer-secret123"}, setupBackend)
	var cpAddressChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", map[string]any{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key", configCPAddress: "FOO"})
	RunOutgoingTestCases(t, cpAddressChannel, newHandler(), cpAddressSendTestCases, []string{"customer-key", "customer-secret123"}, setupBackend)
}
