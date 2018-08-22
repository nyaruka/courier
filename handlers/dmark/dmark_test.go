package dmark

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DM", "2020", "RW", nil),
}

var (
	receiveURL = "/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive = "empty"
	validReceive = "text=Msg&short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=254791541111"
	invalidURN   = "text=Msg&short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=MTN"
	missingText  = "short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=254791541111"
	invalidTS    = "text=Msg&short_code=2020&tstamp=2017-10-26&msisdn=254791541111"

	missingStatus = "id=12345"
	invalidStatus = "id=12345&status=Borked"
	validStatus   = "id=12345&status=1"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"),
		Date: Tp(time.Date(2017, 10, 26, 15, 51, 32, 906335000, time.UTC))},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive Empty", URL: receiveURL, Data: emptyReceive, Status: 400, Response: "field 'msisdn' required"},
	{Label: "Receive Missing Text", URL: receiveURL, Data: missingText, Status: 400, Response: "field 'text' required"},
	{Label: "Receive Invalid TS", URL: receiveURL, Data: invalidTS, Status: 400, Response: "invalid tstamp"},

	{Label: "Status Invalid", URL: statusURL, Status: 400, Data: invalidStatus, Response: "unknown status"},
	{Label: "Status Missing", URL: statusURL, Status: 400, Data: missingStatus, Response: "field 'status' required"},
	{Label: "Status Valid", URL: statusURL, Status: 200, Data: validStatus, Response: `"status":"D"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "6b1c15d3-cba2-46f7-9a25-78265e58057d",
		ResponseBody: `{ "type": "MT", "sms_id": "6b1c15d3-cba2-46f7-9a25-78265e58057d" }`, ResponseStatus: 200,
		Headers:    map[string]string{"Authorization": "Token Authy"},
		PostParams: map[string]string{"text": "Simple Message ☺", "receiver": "250788383383", "sender": "2020", "dlr_url": "https://localhost/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%s"},
		SendPrep:   setSendURL},
	{Label: "Invalid Body",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 200,
		Headers:    map[string]string{"Authorization": "Token Authy"},
		PostParams: map[string]string{"text": "Error Message", "receiver": "250788383383", "sender": "2020"},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		Headers:    map[string]string{"Authorization": "Token Authy"},
		PostParams: map[string]string{"text": "Error Message", "receiver": "250788383383", "sender": "2020"},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		map[string]interface{}{
			courier.ConfigAuthToken: "Authy",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
