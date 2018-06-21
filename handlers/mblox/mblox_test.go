package mblox

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MB", "2020", "BR", map[string]interface{}{"username": "zv-username", "password": "zv-password"}),
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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+12067799294"), Date: Tp(time.Date(2016, 3, 30, 19, 33, 06, 643000000, time.UTC)),
		ExternalID: Sp("OzQ5UqIOdoY8")},

	{Label: "Receive Missing Params", URL: receiveURL, Data: missingParamsRecieve, Status: 400, Response: "missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},

	{Label: "Status Valid", URL: receiveURL, Data: validStatus, Status: 200, Response: `"status":"D"`},
	{Label: "Status Unknown", URL: receiveURL, Data: unknownStatus, Status: 400, Response: `unknown status 'INVALID'`},
	{Label: "Status Missing Batch ID", URL: receiveURL, Data: missingBatchID, Status: 400, Response: "missing one of 'batch_id' or 'status' in request body"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{ "id":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer Password",
		},
		RequestBody: `{"from":"2020","to":["250788383383"],"body":"Simple Message ☺","delivery_report":"per_recipient"}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{ "id":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer Password",
		},
		RequestBody: `{"from":"2020","to":["250788383383"],"body":"I need to keep adding more things to make it work","delivery_report":"per_recipient"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "tel:+250788383383",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{ "id":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer Password",
		},
		RequestBody: `{"from":"2020","to":["250788383383"],"body":"My pic!\nhttps://foo.bar/image.jpg","delivery_report":"per_recipient"}`,
		SendPrep:    setSendURL},
	{Label: "No External Id",
		Text:           "No External ID",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "missing":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Error:          "unable to parse response body from MBlox",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer Password",
		},
		RequestBody: `{"from":"2020","to":["250788383383"],"body":"No External ID","delivery_report":"per_recipient"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		RequestBody:    `{"from":"2020","to":["250788383383"],"body":"Error Message","delivery_report":"per_recipient"}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MB", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
