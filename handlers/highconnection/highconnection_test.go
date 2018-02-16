package highconnection

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HX", "2020", "US", nil),
}

var (
	receiveURL = "/c/hx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	validReceive = receiveURL + "?FROM=+33610346460&TO=5151&MESSAGE=Hello+World&RECEPTION_DATE=2015-04-02T14:26:06"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: validReceive, Status: 200, Response: "Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+33610346460"),
		Date: Tp(time.Date(2015, 04, 02, 14, 26, 06, 0, time.UTC))},
	{Label: "Receive missing params", URL: receiveURL, Status: 400, Response: "validation for 'Message' failed"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
