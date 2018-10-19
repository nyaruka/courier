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
	receiveValidMessage = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=Join&ts=1493735509&id=asdf-asdf&to=24453"
	receiveKIMessage    = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B68673076228&message=Join&ts=1493735509&id=asdf-asdf&to=24453"
	receiveInvalidURN   = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=MTN&message=Join&ts=1493735509&id=asdf-asdf&to=24453"
	receiveEmptyMessage = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?backend=NIG_MTN&sender=%2B2349067554729&message=&ts=1493735509&id=asdf-asdf&to=24453"
	statusNoParams      = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	statusInvalidStatus = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=66"
	statusValid         = "/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/?id=12345&status=4"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), ExternalID: Sp("asdf-asdf"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive KI Message", URL: receiveKIMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+68673076228"), ExternalID: Sp("asdf-asdf"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive Empty Message", URL: receiveEmptyMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp(""), URN: Sp("tel:+2349067554729"), ExternalID: Sp("asdf-asdf"), Date: Tp(time.Date(2017, 5, 2, 14, 31, 49, 0, time.UTC))},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'sender' required"},
	{Label: "Invalid URN", URL: receiveInvalidURN, Data: "empty", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Status No Params", URL: statusNoParams, Status: 400, Response: "field 'status' required"},
	{Label: "Status Invalid Status", URL: statusInvalidStatus, Status: 400, Response: "unknown status '66', must be one of 1,2,4,8,16"},
	{Label: "Status Valid", URL: statusValid, Status: 200, Response: `"status":"S"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("send_url", s.URL)
}

// setSendURLWithQuery takes care of setting the send_url to our test server host
func setSendURLWithQuery(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("send_url", s.URL+"?auth=foo")
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383", HighPriority: false,
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "coding": "", "priority": "",
			"dlr-url": "https://localhost/c/kn/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%d"},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383", HighPriority: false,
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "☺", "to": "+250788383383", "coding": "2", "charset": "utf8", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Smart Encoding",
		Text: "Fancy “Smart” Quotes", URN: "tel:+250788383383", HighPriority: false,
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": `Fancy "Smart" Quotes`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Not Routable",
		Text: "Not Routable", URN: "tel:+250788383383", HighPriority: false,
		Status:       "F",
		ResponseBody: "Not routable. Do not try again.", ResponseStatus: 403,
		URLParams: map[string]string{"text": `Not Routable`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383", HighPriority: false,
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		URLParams: map[string]string{"text": `Error Message`, "to": "+250788383383", "coding": "", "priority": ""},
		SendPrep:  setSendURL},
	{Label: "Custom Params",
		Text: "Custom Params", URN: "tel:+250788383383", HighPriority: true,
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 201,
		URLParams: map[string]string{"text": `Custom Params`, "to": "+250788383383", "coding": "", "priority": "1", "auth": "foo"},
		SendPrep:  setSendURLWithQuery},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", HighPriority: true, Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		URLParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020", "dlr-mask": "27"},
		SendPrep:  setSendURL},
}

var nationalSendTestCases = []ChannelSendTestCase{
	{Label: "National Send",
		Text: "success", URN: "tel:+250788383383", HighPriority: true,
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "success", "to": "788383383", "coding": "", "priority": "1", "dlr-mask": "3"},
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
			"dlr_mask":     "3",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, nationalChannel, newHandler(), nationalSendTestCases, nil)
}
