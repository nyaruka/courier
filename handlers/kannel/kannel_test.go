package kannel

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveNoParams     = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	receiveValidMessage = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=Join&ts=1493735509&id=12345&to=24453"
	statusNoParams      = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	statusInvalidStatus = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=66"
	statusValid         = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), External: Sp("12345"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'message' required"},
	{Label: "Status No Params", URL: statusNoParams, Status: 400, Response: "field 'status' required"},
	{Label: "Status Invalid Status", URL: statusInvalidStatus, Status: 400, Response: "unknown status '66', must be one of 1,2,4,8,16"},
	{Label: "Status Valid", URL: statusValid, Status: 200, Response: "Status Update Accepted"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	channel.(*courier.MockChannel).SetConfig("send_url", server.URL)
}

// setSendURLWithQuery takes care of setting the send_url to our test server host
func setSendURLWithQuery(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	channel.(*courier.MockChannel).SetConfig("send_url", server.URL+"?auth=foo")
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "☺", "to": "+250788383383", "coding": "2", "charset": "utf8", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Smart Encoding",
		Text: "Fancy “Smart” Quotes", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		Error:     "received error sending message",
		URLParams: map[string]string{"text": `Error Message`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Custom Params",
		Text: "Custom Params", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 201,
		URLParams: map[string]string{"text": `Custom Params`, "to": "+250788383383", "coding": "", "priority": "", "auth": "foo"},
		SendPrep:  setSendURLWithQuery},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		URLParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		SendPrep:  setSendURL},
}

var nationalSendTestCases = []ChannelSendTestCase{
	{Label: "National Send",
		Text: "success", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "success", "to": "0788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username"})

	var nationalChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"password":     "Password",
			"username":     "Username",
			"use_national": true,
			"verify_ssl":   false,
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
	RunChannelSendTestCases(t, nationalChannel, NewHandler(), nationalSendTestCases)
}
