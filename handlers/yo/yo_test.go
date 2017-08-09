package yo

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveValidMessage         = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join"
	receiveValidMessageFrom     = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join"
	receiveValidMessageWithDate = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z"
	receiveNoParams             = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveNoSender             = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
	receiveInvalidDate          = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=20170623T123000Z"
	failedNoParams              = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/"
	failedValid                 = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/?id=12345"
	sentValid                   = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/?id=12345"
	invalidStatus               = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/wired/"
	deliveredValid              = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/?id=12345"
	deliveredValidPost          = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "YO", "2020", "US", map[string]interface{}{"username": "yo-username", "password": "yo-password"}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid From", URL: receiveValidMessageFrom, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Message With Date", URL: receiveValidMessageWithDate, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC))},
	{Label: "Receive Valid Message With Time", URL: receiveValidMessageWithTime, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC))},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'text' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "empty", Status: 400, Response: "must have one of 'sender' or 'from' set"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "empty", Status: 400, Response: "invalid date format, must be RFC 3339"},
	{Label: "Failed No Params", URL: failedNoParams, Status: 400, Response: "field 'id' required"},
	{Label: "Failed Valid", URL: failedValid, Status: 200, Response: `{"status":"F"}`},
	{Label: "Invalid Status", URL: invalidStatus, Status: 404, Response: `page not found`},
	{Label: "Sent Valid", URL: sentValid, Status: 200, Response: `{"status":"S"}`},
	{Label: "Delivered Valid", URL: deliveredValid, Status: 200, Response: `{"status":"D"}`},
	{Label: "Delivered Valid Post", URL: deliveredValidPost, Data: "id=12345", Status: 200, Response: `{"status":"D"}`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL1 = server.URL
	sendURL2 = server.URL
	sendURL3 = server.URL
}

var getSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "ybs_autocreate_status=OK", ResponseStatus: 200,
		URLParams: map[string]string{"sms_content": "Simple Message", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:  setSendURL},
	{Label: "Blacklisted",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "F",
		ResponseBody: "ybs_autocreate_status=ERROR&ybs_autocreate_message=256794224665%3ABLACKLISTED", ResponseStatus: 200,
		URLParams: map[string]string{"sms_content": "Simple Message", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:  setSendURL,
		Stopped:   true},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "ybs_autocreate_status=OK", ResponseStatus: 200,
		URLParams: map[string]string{"sms_content": "☺", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "Error", ResponseStatus: 401,
		Error:     "received non 200 status: 401",
		URLParams: map[string]string{"sms_content": `Error Message`, "destinations": string("250788383383")},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: "ybs_autocreate_status=OK", ResponseStatus: 200,
		URLParams: map[string]string{"sms_content": "My pic!\nhttps://foo.bar/image.jpg", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:  setSendURL},
}

func TestSending(t *testing.T) {
	var getChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "YO", "2020", "US", map[string]interface{}{"username": "yo-username", "password": "yo-password"})

	RunChannelSendTestCases(t, getChannel, NewHandler(), getSendTestCases)
}
