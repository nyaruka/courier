package clickmobile

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/dates"
)

var (
	receiveURL = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	notXML = "empty"

	validReceive = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	invalidURNReceive = `<request>
	<shortCode>2020</shortCode>
	<mobile>MTN</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	validReceiveEmptyText = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text></text>
	</request>`

	validMissingText = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	</request>`

	validMissingReferenceID = `<request>
	<shortCode>2020</shortCode>
	<mobile>265990099333</mobile>
	<text>Join</text>
	</request>`

	missingShortcode = `<request>
	<mobile>265990099333</mobile>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`

	missingMobile = `<request>
	<shortCode>2020</shortCode>
	<referenceID>1232434354</referenceID>
	<text>Join</text>
	</request>`
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CM", "2020", "MW", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: validReceive, Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+265990099333"), ExternalID: Sp("1232434354")},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURNReceive, Status: 400, Response: "phone number supplied is not a number"},

	{Label: "Receive valid with empty text", URL: receiveURL, Data: validReceiveEmptyText, Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("tel:+265990099333"), ExternalID: Sp("1232434354")},
	{Label: "Receive valid missing text", URL: receiveURL, Data: validMissingText, Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("tel:+265990099333"), ExternalID: Sp("1232434354")},

	{Label: "Receive valid missing referenceID", URL: receiveURL, Data: validMissingReferenceID, Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+265990099333"), ExternalID: Sp("")},

	{Label: "Missing Shortcode", URL: receiveURL, Data: missingShortcode, Status: 400, Response: "missing parameters, must have 'mobile' and 'shortcode'"},
	{Label: "Missing Mobile", URL: receiveURL, Data: missingMobile, Status: 400, Response: "missing parameters, must have 'mobile' and 'shortcode'"},
	{Label: "Receive invalid XML", URL: receiveURL, Data: notXML, Status: 400, Response: "unable to parse request XML"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig(courier.ConfigSendURL, s.URL)
	sendURL = s.URL

}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`, ResponseStatus: 200,
		RequestBody: `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"Simple Message"}`,
		Headers:     map[string]string{"Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: `{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`, ResponseStatus: 200,
		RequestBody: `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"☺"}`,
		Headers:     map[string]string{"Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{"code":"001","desc":"Database SQL Error"}`, ResponseStatus: 401,
		RequestBody: `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"Error Message"}`,
		Headers:     map[string]string{"Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{"code":"000","desc":"Operation successful.","data":{"new_record_id":"9"}}`, ResponseStatus: 200,
		RequestBody: `{"app_id":"001-app","org_id":"001-org","user_id":"Username","timestamp":"20180411182430","auth_key":"3e1347ddb444d13aa23d11e097602be0","operation":"send","reference":"10","message_type":"1","src_address":"2020","dst_address":"+250788383383","message":"My pic!\nhttps://foo.bar/image.jpg"}`,
		Headers:     map[string]string{"Content-Type": "application/json"},
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CM", "2020", "MW",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
			"app_id":   "001-app",
			"org_id":   "001-org",
			"send_url": "SendURL",
		},
	)

	// mock time so we can have predictable MD5 hashes
	dates.SetNowSource(dates.NewFixedNowSource(time.Date(2018, 4, 11, 18, 24, 30, 123456000, time.UTC)))

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
