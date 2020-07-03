package clickmobile

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils/dates"
)

var (
	receiveValidMessage = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join"
	receiveNoParams     = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN          = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=MTN&text=Join"
	receiveNoSender     = "/c/cm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CM", "2020", "MW", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: invalidURN, Data: "", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", Status: 400, Response: "field 'from' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", Status: 400, Response: "field 'from' required"},

	{Label: "Receive Valid Message", URL: receiveNoParams, Data: "from=%2B2349067554729&text=Join", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: receiveNoParams, Data: "from=MTN&text=Join", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'from' required"},
	{Label: "Receive No Sender", URL: receiveNoParams, Data: "text=Join", Status: 400, Response: "field 'from' required"},
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
