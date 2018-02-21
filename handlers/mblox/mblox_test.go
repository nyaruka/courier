package mblox

import (
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
	{Label: "Receive Valid", URL: receiveURL, Data: missingParamsRecieve, Status: 400, Response: "missing one of 'id', 'from', 'to', 'body' or 'received_at' in request body"},

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
