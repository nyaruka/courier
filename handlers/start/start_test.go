package start

import (
	"time"
	"testing"
	
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"

)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ST", "2020", "UA", map[string]interface{}{"username": "st-username", "password": "st-password"}),
}

var (
	receiveURL = "/c/st/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	
	notXML = "empty"

	validReceive = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	validReceiveEmptyText = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8"></body>
	</message>`

	missingRequestID = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" />
	<from>+250788123123</from>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`

	missingFrom = `<message>
	<service type="sms" timestamp="1450450974" auth="asdfasdf" request_id="msg1"/>
	<to>1515</to>
	<body content-type="content-type" encoding="utf8">Hello World</body>
	</message>`



		


)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+250788123123"), Date: Tp(time.Date(2015, 12, 18, 15, 02, 54, 0, time.UTC))},
	{Label: "Receive Valid with empty Text", URL: receiveURL, Data: validReceiveEmptyText, Status: 200, Response: "Message Accepted",
		Text: Sp(""), URN: Sp("tel:+250788123123")},

	{Label: "Receive missing Request ID", URL: receiveURL, Data: missingRequestID, Status: 400, Response: "Error"},
	{Label: "Receive missing From", URL: receiveURL, Data: missingFrom, Status: 400, Response: "Error"},
	{Label: "Invalid XML", URL: receiveURL, Data: notXML, Status: 400, Response: "Error"},

}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}

