package africastalking

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US", nil),
}

var (
	receiveURL = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive = "empty"
	validReceive = "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111"
	missingText  = "linkId=03090445075804249226&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111"

	missingStatus = "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7"
	invalidStatus = "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Borked"
)

var testCases = []ChannelTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"), External: Sp("ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3"),
		Date: Tp(time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC))},
	{Label: "Receive Empty", URL: receiveURL, Data: emptyReceive, Status: 400, Response: "field 'id' required"},
	{Label: "Receive Missing Text", URL: receiveURL, Data: missingText, Status: 400, Response: "field 'text' required"},

	{Label: "Status Invalid", URL: statusURL, Status: 400, Data: invalidStatus, Response: "unknown status"},
	{Label: "Status Missing", URL: statusURL, Status: 400, Data: missingStatus, Response: "field 'status' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
