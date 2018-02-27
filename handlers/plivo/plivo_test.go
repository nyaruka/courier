package plivo

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "MY", nil),
}

var (
	receiveURL = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive   = "To=2020&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	invalidAddress = "To=1515&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	missingParams  = "From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"

	validStatus     = "MessageUUID=12345&status=delivered&From=%2B60124361111&To=2020"
	validSentStatus = "ParentMessageUUID=12345&status=sent&MessageUUID=123&From=%2B60124361111&To=2020"
	unknownStatus   = "MessageUUID=12345&status=UNKNOWN&From=%2B60124361111&To=2020"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"),
		ExternalID: Sp("abc1234")},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, Status: 400, Response: "invalid to number [1515], expecting [2020]"},
	{Label: "Missing Params", URL: receiveURL, Data: missingParams, Status: 400, Response: "Field validation for 'To' failed"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `"status":"D"`},
	{Label: "Sent Status", URL: statusURL, Data: validSentStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Unkown Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: `ignoring unknown status 'UNKNOWN'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}
