package africastalking

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US", nil),
}

var (
	receiveURL = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/at/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	emptyReceive          = "empty"
	validReceive          = "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111"
	validOtherDateReceive = "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03+06%3A04%3A45&from=%2B254791541111"
	invalidURN            = "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=MTN"
	missingText           = "linkId=03090445075804249226&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04%3A45Z&from=%2B254791541111"
	invalidDate           = "linkId=03090445075804249226&text=Msg&to=21512&id=ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3&date=2017-05-03T06%3A04&from=%2B254791541111"

	missingStatus = "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7"
	invalidStatus = "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Borked"
	validStatus   = "id=ATXid_dda018a640edfcc5d2ce455de3e4a6e7&status=Success"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"), ExternalID: Sp("ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3"),
		Date: Tp(time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC))},
	{Label: "Receive Valid", URL: receiveURL, Data: validOtherDateReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"), ExternalID: Sp("ec9adc86-51d5-4bc8-8eb0-d8ab0bb53dc3"),
		Date: Tp(time.Date(2017, 5, 3, 06, 04, 45, 0, time.UTC))},
	{Label: "Receive Empty", URL: receiveURL, Data: emptyReceive, Status: 400, Response: "field 'id' required"},
	{Label: "Receive Missing Text", URL: receiveURL, Data: missingText, Status: 400, Response: "field 'text' required"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Invalid Date", URL: receiveURL, Data: invalidDate, Status: 400, Response: "invalid date format"},
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
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`, ResponseStatus: 200,
		Headers:    map[string]string{"apikey": "KEY"},
		PostParams: map[string]string{"message": "Simple Message ☺", "username": "Username", "to": "+250788383383", "from": "2020"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`, ResponseStatus: 200,
		PostParams: map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg"},
		SendPrep:   setSendURL},
	{Label: "No External Id",
		Text: "No External ID", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "SMSMessageData": {"Recipients": [{"status": "Failed" }] } }`, ResponseStatus: 200,
		PostParams: map[string]string{"message": `No External ID`},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		PostParams: map[string]string{"message": `Error Message`},
		SendPrep:   setSendURL},
}

var sharedSendTestCases = []ChannelSendTestCase{
	{Label: "Shared Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "SMSMessageData": {"Recipients": [{"status": "Success", "messageId": "1002"}] } }`, ResponseStatus: 200,
		Headers:    map[string]string{"apikey": "KEY"},
		PostParams: map[string]string{"message": "Simple Message ☺", "username": "Username", "to": "+250788383383", "from": ""},
		SendPrep:   setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
		})
	var sharedChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigAPIKey:   "KEY",
			configIsShared:         true,
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, sharedChannel, newHandler(), sharedSendTestCases, nil)
}
