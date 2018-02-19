package chikka

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CK", "2020", "US", nil),
}

var (
	receiveURL        = "/c/ck/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	validReceive      = "message_type=incoming&mobile_number=639178020779&request_id=4004&message=Hello+World&timestamp=1457670059.69"
	validSentStatus   = "message_type=outgoing&message_id=10&status=SENT"
	validFailedStatus = "message_type=outgoing&message_id=10&status=FAILED"
	invalidStatus     = "message_type=outgoing&message_id=10&status=UNKNOWN"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+639178020779"), ExternalID: Sp("4004"),
		Date: Tp(time.Date(2016, 03, 11, 04, 20, 59, 690000128, time.UTC))},

	{Label: "Ignore Invalid message_type", URL: receiveURL, Data: "message_type=invalid", Status: 200, Response: "unknown message_type request"},
	{Label: "Status Sent Valid", URL: receiveURL, Data: validSentStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Status Failed Valid", URL: receiveURL, Data: validFailedStatus, Status: 200, Response: `"status":"F"`},
	{Label: "Status Invalid", URL: receiveURL, Data: invalidStatus, Status: 400, Response: `must be either 'SENT' or 'FAILED'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
