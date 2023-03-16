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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", map[string]interface{}{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"}),
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

var validStatus = `{
	"requestId": "req445454",
	"clientCorrelator": "string",
	"deliveryStatus": [
		{
			"receiverAddress": "27568942200",
			"status": "DeliveredToTerminal"
		}
	]
}
`

var testCases = []ChannelHandleTestCase{
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
		Label:                "Receive Valid Status",
		URL:                  statusURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedMsgStatus:    courier.MsgDelivered,
		ExpectedExternalID:   "req445454",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
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
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Simple Message ☺"}`,
		SendPrep:            setSendURL},
	{Label: "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "OzYDlvf3SQVc",
		MockResponseBody:   `{ "transactionId":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer ACCESS_TOKEN",
		},
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"I need to keep adding more things to make it work"}`,
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
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"My pic!\nhttps://foo.bar/image.jpg"}`,
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
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"No External ID"}`,
		SendPrep:            setSendURL},
	{Label: "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		ExpectedMsgStatus:   "E",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"senderAddress":"2020","receiverAddress":["250788383383"],"message":"Error Message"}`,
		SendPrep:            setSendURL},
}

func setupBackend(mb *test.MockBackend) {
	// ensure there's a cached access token
	rc := mb.RedisPool().Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", map[string]interface{}{courier.ConfigAuthToken: "customer-secret123", courier.ConfigAPIKey: "customer-key"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"customer-key", "customer-secret123"}, setupBackend)
}
