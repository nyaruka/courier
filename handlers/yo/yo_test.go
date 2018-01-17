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
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "YO", "2020", "US", map[string]interface{}{"username": "yo-username", "password": "yo-password"}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid From", URL: receiveValidMessageFrom, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Message With Date", URL: receiveValidMessageWithDate, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC))},
	{Label: "Receive Valid Message With Time", URL: receiveValidMessageWithTime, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC))},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", Status: 400, Response: "field 'text' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", Status: 400, Response: "must have one of 'sender' or 'from' set"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "", Status: 400, Response: "invalid date format, must be RFC 3339"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL1 = s.URL
	sendURL2 = s.URL
	sendURL3 = s.URL
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
	{Label: "Errored wrong authorization",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		Error:        "received error from Yo! API",
		ResponseBody: "ybs_autocreate_status=ERROR&ybs_autocreate_message=YBS+AutoCreate+Subsystem%3A+Access+denied+due+to+wrong+authorization+code", ResponseStatus: 200,
		URLParams: map[string]string{"sms_content": "Simple Message", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:  setSendURL},
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
