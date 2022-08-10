package yo

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveValidMessage         = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=2349067554729&message=Join"
	receiveValidMessageFrom     = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&message=Join"
	receiveValidMessageWithDate = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&message=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&message=Join&time=2017-06-23T12:30:00Z"
	receiveInvalidURN           = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=MTN&message=Join"
	receiveNoParams             = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveNoSender             = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?message=Join"
	receiveInvalidDate          = "/c/yo/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&message=Join&time=20170623T123000Z"
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
	{Label: "Invalid URN", URL: receiveInvalidURN, Data: "", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", Status: 400, Response: "must have one of 'sender' or 'from'"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", Status: 400, Response: "must have one of 'sender' or 'from'"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "", Status: 400, Response: "invalid date format, must be RFC 3339"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURLs = []string{s.URL}
}

var getSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: "ybs_autocreate_status=OK", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"sms_content":  "Simple Message",
			"destinations": "250788383383",
			"ybsacctno":    "yo-username",
			"password":     "yo-password",
			"origin":       "2020"},
		SendPrep: setSendURL},
	{Label: "Blacklisted",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "F",
		MockResponseBody: "ybs_autocreate_status=ERROR&ybs_autocreate_message=256794224665%3ABLACKLISTED", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"sms_content": "Simple Message", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:          setSendURL,
		ExpectedStopEvent: true},
	{Label: "Errored wrong authorization",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: "ybs_autocreate_status=ERROR&ybs_autocreate_message=YBS+AutoCreate+Subsystem%3A+Access+denied+due+to+wrong+authorization+code", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"sms_content": "Simple Message", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:          setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: "ybs_autocreate_status=OK", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"sms_content": "☺", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:          setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: "Error", MockResponseStatus: 401,
		ExpectedURLParams: map[string]string{"sms_content": `Error Message`, "destinations": string("250788383383")},
		SendPrep:          setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedStatus:   "W",
		MockResponseBody: "ybs_autocreate_status=OK", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"sms_content": "My pic!\nhttps://foo.bar/image.jpg", "destinations": string("250788383383"), "origin": "2020"},
		SendPrep:          setSendURL},
}

func TestSending(t *testing.T) {
	var getChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "YO", "2020", "US", map[string]interface{}{"username": "yo-username", "password": "yo-password"})

	RunChannelSendTestCases(t, getChannel, newHandler(), getSendTestCases, nil)
}
