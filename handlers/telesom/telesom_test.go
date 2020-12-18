package telesom

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/dates"
)

var (
	receiveValidMessage = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=%2B2349067554729&msg=Join"
	receiveNoParams     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN          = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=MTN&msg=Join"
	receiveNoSender     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?msg=Join"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "SO", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: invalidURN, Data: "", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", Status: 400, Response: "field 'mobile' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", Status: 400, Response: "field 'mobile' required"},

	{Label: "Receive Valid Message", URL: receiveNoParams, Data: "mobile=%2B2349067554729&msg=Join", Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Invalid URN", URL: receiveNoParams, Data: "mobile=MTN&msg=Join", Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", Status: 400, Response: "field 'mobile' required"},
	{Label: "Receive No Sender", URL: receiveNoParams, Data: "msg=Join", Status: 400, Response: "field 'mobile' required"},
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
		Text: "Simple Message", URN: "tel:+252788383383",
		Status:       "W",
		ResponseBody: "<return>Success</return>", ResponseStatus: 200,
		URLParams: map[string]string{"msg": "Simple Message", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "D69BB824F88F20482B94ECF3822EBD84"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+252788383383",
		Status:       "W",
		ResponseBody: "<return>Success</return>", ResponseStatus: 200,
		URLParams: map[string]string{"msg": "☺", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "60421A7D99BD79FE02697D567315AD0E"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+252788383383",
		Status:       "E",
		ResponseBody: "<return>error</return>", ResponseStatus: 401,
		URLParams: map[string]string{"msg": `Error Message`, "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "3F1E492B2186551570F24C2F07D5D7E2"},
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:  setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+252788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `<return>Success</return>`, ResponseStatus: 200,
		URLParams: map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "DBE569579FD899628C17254ECCE15DB7"},
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
