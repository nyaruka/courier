package shaqodoon

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var (
	receiveValidMessage         = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join"
	receiveBadlyEscaped         = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=+252999999999&text=Join"
	receiveInvalidURN           = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=MTN&text=Join"
	receiveEmptyMessage         = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text="
	receiveValidMessageWithDate = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z"
	receiveNoParams             = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveNoSender             = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
	receiveInvalidDate          = "/c/sq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join&time=20170623T123000Z"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SQ", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Badly Escaped", URL: receiveBadlyEscaped, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+252999999999")},
	{Label: "Receive Empty Message", URL: receiveEmptyMessage, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp(""), ExpectedURN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Message With Date", URL: receiveValidMessageWithDate, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729"), ExpectedDate: Tp(time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC))},
	{Label: "Receive Valid Message With Time", URL: receiveValidMessageWithTime, Data: "empty", ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: Sp("tel:+2349067554729"), ExpectedDate: Tp(time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC))},
	{Label: "Receive invalid URN", URL: receiveInvalidURN, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "field 'from' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "field 'from' required"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "empty", ExpectedStatus: 400, ExpectedResponse: "invalid date format, must be RFC 3339"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*test.MockChannel).SetConfig(courier.ConfigSendURL, s.URL)
}

var getSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: "0: Accepted for delivery", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "Simple Message", "to": "250788383383", "from": "2020"},
		SendPrep:          setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "W",
		MockResponseBody: "0: Accepted for delivery", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "☺", "to": "250788383383", "from": "2020"},
		SendPrep:          setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		ExpectedStatus:   "E",
		MockResponseBody: "1: Unknown channel", MockResponseStatus: 401,
		ExpectedURLParams: map[string]string{"msg": `Error Message`, "to": "250788383383"},
		SendPrep:          setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedStatus:   "W",
		MockResponseBody: `0: Accepted for delivery`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020"},
		SendPrep:          setSendURL},
}

func TestSending(t *testing.T) {
	var getChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SQ", "2020", "US",
		map[string]interface{}{
			courier.ConfigSendURL:  "SendURL",
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username"})

	RunChannelSendTestCases(t, getChannel, newHandler(), getSendTestCases, nil)
}
