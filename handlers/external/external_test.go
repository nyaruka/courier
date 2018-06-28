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
	receiveValidNoPlus          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=2349067554729&text=Join"
	receiveValidMessageWithDate = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&date=2017-06-23T12:30:00.500Z"
	receiveValidMessageWithTime = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=2017-06-23T12:30:00Z"
	receiveNoParams             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN                  = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=MTN12345678901234567&text=Join"
	receiveNoSender             = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?text=Join"
	receiveInvalidDate          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?sender=%2B2349067554729&text=Join&time=20170623T123000Z"
	failedNoParams              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/"
	failedValid                 = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/failed/?id=12345"
	sentValid                   = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/?id=12345"
	invalidStatus               = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/wired/"
	deliveredValid              = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/?id=12345"
	deliveredValidPost          = "/c/ex/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Post", URL: receiveNoParams, Data: "sender=%2B2349067554729&text=Join", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid From", URL: receiveValidMessageFrom, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Country Parse", URL: receiveValidNoPlus, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive Valid Message With Date", URL: receiveValidMessageWithDate, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, int(500*time.Millisecond), time.UTC))},
	{Label: "Receive Valid Message With Time", URL: receiveValidMessageWithTime, Data: "empty", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729"), Date: Tp(time.Date(2017, 6, 23, 12, 30, 0, 0, time.UTC))},
	{Label: "Invalid URN", URL: invalidURN, Data: "empty", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'text' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "empty", Status: 400, Response: "must have one of 'sender' or 'from' set"},
	{Label: "Receive Invalid Date", URL: receiveInvalidDate, Data: "empty", Status: 400, Response: "invalid date format, must be RFC 3339"},
	{Label: "Failed No Params", URL: failedNoParams, Status: 400, Response: "field 'id' required"},
	{Label: "Failed Valid", URL: failedValid, Status: 200, Response: `"status":"F"`},
	{Label: "Invalid Status", URL: invalidStatus, Status: 404, Response: `page not found`},
	{Label: "Sent Valid", URL: sentValid, Status: 200, Response: `"status":"S"`},
	{Label: "Delivered Valid", URL: deliveredValid, Status: 200, Data: "nothing", Response: `"status":"D"`},
	{Label: "Delivered Valid Post", URL: deliveredValidPost, Data: "id=12345", Status: 200, Response: `"status":"D"`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	// this is actually a path, which we'll combine with the test server URL
	sendURL := c.StringConfigForKey("send_path", "")
	sendURL, _ = utils.AddURLPath(s.URL, sendURL)
	c.(*courier.MockChannel).SetConfig(courier.ConfigSendURL, sendURL)
}

var longSendTestCases = []ChannelSendTestCase{
	{Label: "Long Send",
		Text: "This is a long message that will be longer than 30....... characters", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "characters", "to": "+250788383383", "from": "2020"},
		SendPrep:  setSendURL},
}

var getSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		URLParams: map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		URLParams: map[string]string{"text": `Error Message`, "to": "+250788383383"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		URLParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
}

var postSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		PostParams: map[string]string{"text": "Simple Message", "to": "+250788383383", "from": "2020"},
		Headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:   setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		PostParams: map[string]string{"text": "☺", "to": "+250788383383", "from": "2020"},
		Headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:   setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		PostParams: map[string]string{"text": `Error Message`, "to": "+250788383383"},
		Headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:   setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		PostParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "+250788383383", "from": "2020"},
		Headers:    map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:   setSendURL},
}

var jsonSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		RequestBody: `{ "to":"+250788383383", "text":"Simple Message", "from":"2020" }`,
		Headers:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: `☺ "hi!"`, URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		RequestBody: `{ "to":"+250788383383", "text":"☺ \"hi!\"", "from":"2020" }`,
		Headers:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		RequestBody: `{ "to":"+250788383383", "text":"Error Message", "from":"2020" }`,
		Headers:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		RequestBody: `{ "to":"+250788383383", "text":"My pic!\nhttps://foo.bar/image.jpg", "from":"2020" }`,
		Headers:     map[string]string{"Authorization": "Token ABCDEF", "Content-Type": "application/json"},
		SendPrep:    setSendURL},
}

var xmlSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		RequestBody: `<msg><to>+250788383383</to><text>Simple Message</text><from>2020</from></msg>`,
		Headers:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: `☺`, URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "0: Accepted for delivery", ResponseStatus: 200,
		RequestBody: `<msg><to>+250788383383</to><text>☺</text><from>2020</from></msg>`,
		Headers:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "1: Unknown channel", ResponseStatus: 401,
		RequestBody: `<msg><to>+250788383383</to><text>Error Message</text><from>2020</from></msg>`,
		Headers:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `0: Accepted for delivery`, ResponseStatus: 200,
		RequestBody: `<msg><to>+250788383383</to><text>My pic!&#xA;https://foo.bar/image.jpg</text><from>2020</from></msg>`,
		Headers:     map[string]string{"Content-Type": "text/xml; charset=utf-8"},
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var getChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}",
			courier.ConfigSendMethod: http.MethodGet})

	var postChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"send_path":              "",
			courier.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}",
			courier.ConfigSendMethod: http.MethodPost})

	var jsonChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"send_path":                     "",
			courier.ConfigSendBody:          `{ "to":{{to}}, "text":{{text}}, "from":{{from}} }`,
			courier.ConfigContentType:       contentJSON,
			courier.ConfigSendMethod:        http.MethodPost,
			courier.ConfigSendAuthorization: "Token ABCDEF",
		})

	var xmlChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"send_path":               "",
			courier.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from></msg>`,
			courier.ConfigContentType: contentXML,
			courier.ConfigSendMethod:  http.MethodPut,
		})

	RunChannelSendTestCases(t, getChannel, newHandler(), getSendTestCases, nil)
	RunChannelSendTestCases(t, postChannel, newHandler(), postSendTestCases, nil)
	RunChannelSendTestCases(t, jsonChannel, newHandler(), jsonSendTestCases, nil)
	RunChannelSendTestCases(t, xmlChannel, newHandler(), xmlSendTestCases, nil)

	var getChannel30IntLength = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"max_length":             30,
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}",
			courier.ConfigSendMethod: http.MethodGet})

	var getChannel30StrLength = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		map[string]interface{}{
			"max_length":             "30",
			"send_path":              "?to={{to}}&text={{text}}&from={{from}}",
			courier.ConfigSendMethod: http.MethodGet})

	RunChannelSendTestCases(t, getChannel30IntLength, newHandler(), longSendTestCases, nil)
	RunChannelSendTestCases(t, getChannel30StrLength, newHandler(), longSendTestCases, nil)
}
