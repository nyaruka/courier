package zenvia

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZV", "2020", "BR", map[string]interface{}{"account": "zv_account", "code": "zv-code"}),
}

var (
	receiveURL = "/c/zv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/zv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive = "empty"
	validReceive = "msg=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=03%2F05%2F2017%2006%3A04%3A45&from=%2B254791541111"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"), Date: Tp(time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC))},

	{Label: "Receive Empty", URL: receiveURL, Data: emptyReceive, Status: 400, Response: "field 'id' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
