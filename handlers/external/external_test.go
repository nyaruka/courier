package external

import (
	"net/http/httptest"
	"testing"
	"time"

	"net/http"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

var (
	receiveValidMessage         = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join"
	receiveValidMessageFrom     = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&text=Join"
	receiveValidMessageWithDate = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z"
	receiveNoParams             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveNoSender             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
	receiveInvalidDate          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=20170623T123000Z"
	failedNoParams              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/"
	failedValid                 = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/?id=12345"
	sentValid                   = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/?id=12345"
	deliveredValid              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/?id=12345"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US", nil),
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
	{Label: "Sent Valid", URL: sentValid, Status: 200, Response: `{"status":"S"}`},
	{Label: "Delivered Valid", URL: deliveredValid, Status: 200, Response: `{"status":"D"}`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(server *httptest.Server, channel courier.Channel, msg *courier.Msg) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := channel.StringConfigForKey("send_path", "")
	sendURL, _ = utils.AddURLPath(server.URL, sendURL)
	channel.(*courier.MockChannel).SetConfig(courier.ConfigSendURL, sendURL)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		SendPrep:  setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		Error:     "received non 200 status: 401",
		URLParams: map[string]string{"text": `Error Message`, "to": "+250788383383"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		URLParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		SendPrep:  setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}",
			courier.ConfigSendMethod: http.MethodGet})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
