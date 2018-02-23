package macrokiosk

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MK", "2020", "MY", nil),
}

var (
	receiveURL = "/c/mk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/mk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive         = "shortcode=2020&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	validLongcodeReceive = "longcode=2020&msisdn=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	missingParamsReceive = "from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	invalidParamsReceive = "longcode=2020&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	invalidAddress       = "shortcode=1515&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"

	validStatus      = "msgid=12345&status=ACCEPTED"
	processingStatus = "msgid=12345&status=PROCESSING"
	unknownStatus    = "msgid=12345&status=UNKNOWN"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Receive Valid via GET", URL: receiveURL + "?" + validReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Receive Valid", URL: receiveURL, Data: validLongcodeReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Missing Params", URL: receiveURL, Data: missingParamsReceive, Status: 400, Response: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Params", URL: receiveURL, Data: invalidParamsReceive, Status: 400, Response: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, Status: 400, Response: "invalid to number [1515], expecting [2020]"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Wired Status", URL: statusURL, Data: processingStatus, Status: 200, Response: `"status":"W"`},
	{Label: "Wired Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: `ignoring unknown status 'UNKNOWN'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
