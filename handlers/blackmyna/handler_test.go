package blackmyna

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []*courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BM", "2020", "US", nil),
}

var (
	receiveURL = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/bm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive = receiveURL + ""
	validReceive = receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111&text=Msg"
	missingText  = receiveURL + "?to=3344&smsc=ncell&from=%2B9779814641111"

	missingStatus = statusURL + "?"
	invalidStatus = statusURL + "?id=bmID&status=13"
)

var testCases = []ChannelTestCase{
	{Label: "Receive Valid", URL: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+9779814641111")},
	{Label: "Receive Empty", URL: emptyReceive, Status: 400, Response: "field 'text' required"},
	{Label: "Receive Missing Text", URL: missingText, Status: 400, Response: "field 'text' required"},

	{Label: "Status Invalid", URL: invalidStatus, Status: 400, Response: "unknown status"},
	{Label: "Status Missing", URL: missingStatus, Status: 400, Response: "field 'status' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
