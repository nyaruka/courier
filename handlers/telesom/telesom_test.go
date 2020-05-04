package telesom

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils/dates"
)

var (
	receiveValidMessage = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=%2B2349067554729&msg=Join"
	receiveNoParams     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN          = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?from=MTN&msg=Join"
	receiveNoSender     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?msg=Join"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: invalidURN, Data: "", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", Status: 400, Response: "field 'from' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", Status: 400, Response: "field 'from' required"},

	{Label: "Receive Valid Message", URL: receiveNoParams, Data: "from=%2B2349067554729&msg=Join", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: receiveNoParams, Data: "from=MTN&msg=Join", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'from' required"},
	{Label: "Receive No Sender", URL: receiveNoParams, Data: "msg=Join", Status: 400, Response: "field 'from' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig(courier.ConfigSendURL, s.URL)
	sendURL = s.URL

}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "<return>Success</return>", ResponseStatus: 200,
		URLParams: map[string]string{"msg": "Simple Message", "to": "250788383383", "from": "2020", "username": "Username", "password": "Password", "key": "702FDCC27F2FF04CEB6EF4E1545B8C94"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: "<return>Success</return>", ResponseStatus: 200,
		URLParams: map[string]string{"msg": "☺", "to": "250788383383", "from": "2020", "username": "Username", "password": "Password", "key": "B9C70F93DAE834477E107A128FEA04D4"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: "<return>error</return>", ResponseStatus: 401,
		URLParams: map[string]string{"msg": `Error Message`, "to": "250788383383", "from": "2020", "username": "Username", "password": "Password", "key": "C9F78FC4CC9A416C57AB0A3F208EDF49"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `<return>Success</return>`, ResponseStatus: 200,
		URLParams: map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "username": "Username", "password": "Password", "key": "1D7100B3F9D3249D1A92A0841AD8F543"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "US",
		map[string]interface{}{
			"password": "Password",
			"username": "Username",
			"secret":   "secret",
			"send_url": "SendURL",
		},
	)

	// mock time so we can have predictable MD5 hashes
	dates.SetNowSource(dates.NewFixedNowSource(time.Date(2018, 4, 11, 18, 24, 30, 123456000, time.UTC)))

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
