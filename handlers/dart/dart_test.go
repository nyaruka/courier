package dart

import (
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var daTestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID", nil),
}

var h9TestChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "H9", "2020", "ID", nil),
}

var (
	daReceiveURL = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	daStatusURL  = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	h9ReceiveURL = "/c/h9/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	h9StatusURL  = "/c/h9/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	validMessage = "?userid=testusr&password=test&original=6289881134560&sendto=2020&message=Msg"
	validStatus = "?status=10&messageid=12345"
	badStatus = "?status=foo&messageid=12345"
	badStatusMessageID = "?status=10&messageid=abc"
	missingStatus = "?messageid=12345"

	validDAReceive = daReceiveURL + validMessage
	validDAStatus  = daStatusURL + validStatus
	missingDAStatus = daStatusURL + missingStatus
	badDAStatus = daStatusURL + badStatus
	badDAStatusMessageID = daStatusURL + badStatusMessageID

	validH9Receive = h9ReceiveURL + validMessage
	validH9Status = h9StatusURL + validStatus
	missingH9Status = h9StatusURL + missingStatus
	badH9Status = h9StatusURL + badStatus
	badH9StatusMessageID = h9StatusURL + badStatusMessageID

)

var daTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validDAReceive, Status: 200, Response: "000",
		Text: Sp("Msg"), URN: Sp("tel:+6289881134560")},
	{Label: "Valid Status", URL: validDAStatus, Status: 200, Response: "000"},
	{Label: "Missing Status", URL: missingDAStatus, Status: 400, Response: "parameters messageid and status should not be null"},
	{Label: "Missing Status", URL: badDAStatus, Status: 400, Response: "parsing failed: status 'foo' is not an integer"},
	{Label: "Missing Status", URL: badDAStatusMessageID, Status: 400, Response: "parsing failed: messageid 'abc' is not an integer"},
}

var h9TestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: validH9Receive, Status: 200, Response: "000",
		Text: Sp("Msg"), URN: Sp("tel:+6289881134560")},
	{Label: "Valid Status", URL: validH9Status, Status: 200, Response: "000"},
	{Label: "Missing Status", URL: missingH9Status, Status: 400, Response: "parameters messageid and status should not be null"},
	{Label: "Missing Status", URL: badH9Status, Status: 400, Response: "parsing failed: status 'foo' is not an integer"},
	{Label: "Missing Status", URL: badH9StatusMessageID, Status: 400, Response: "parsing failed: messageid 'abc' is not an integer"},

}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, daTestChannels, NewHandler("DA", "DartMedia"), daTestCases)
	RunChannelTestCases(t, h9TestChannels, NewHandler("H9", "Hub9"), h9TestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, daTestChannels, NewHandler("DA", "DartMedia"), daTestCases)
	RunChannelBenchmarks(b, h9TestChannels, NewHandler("H9", "Hub9"), h9TestCases)
}


