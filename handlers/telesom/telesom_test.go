package telesom

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/dates"
)

var (
	receiveValidMessage = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=%2B2349067554729&msg=Join"
	receiveNoParams     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	invalidURN          = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?mobile=MTN&msg=Join"
	receiveNoSender     = "/c/ts/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/?msg=Join"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "SO", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveValidMessage, Data: "", ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: "tel:+2349067554729"},
	{Label: "Invalid URN", URL: invalidURN, Data: "", ExpectedRespStatus: 400, ExpectedRespBody: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "", ExpectedRespStatus: 400, ExpectedRespBody: "field 'mobile' required"},
	{Label: "Receive No Sender", URL: receiveNoSender, Data: "", ExpectedRespStatus: 400, ExpectedRespBody: "field 'mobile' required"},

	{Label: "Receive Valid Message", URL: receiveNoParams, Data: "mobile=%2B2349067554729&msg=Join", ExpectedRespStatus: 200, ExpectedRespBody: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: "tel:+2349067554729"},
	{Label: "Invalid URN", URL: receiveNoParams, Data: "mobile=MTN&msg=Join", ExpectedRespStatus: 400, ExpectedRespBody: "phone number supplied is not a number"},
	{Label: "Receive No Params", URL: receiveNoParams, Data: "empty", ExpectedRespStatus: 400, ExpectedRespBody: "field 'mobile' required"},
	{Label: "Receive No Sender", URL: receiveNoParams, Data: "msg=Join", ExpectedRespStatus: 400, ExpectedRespBody: "field 'mobile' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*test.MockChannel).SetConfig(courier.ConfigSendURL, s.URL)
	sendURL = s.URL

}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+252788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "<return>Success</return>", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "Simple Message", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "D69BB824F88F20482B94ECF3822EBD84"},
		ExpectedHeaders:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:          setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+252788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  "<return>Success</return>", MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "☺", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "60421A7D99BD79FE02697D567315AD0E"},
		ExpectedHeaders:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:          setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+252788383383",
		ExpectedMsgStatus: "E",
		MockResponseBody:  "<return>error</return>", MockResponseStatus: 401,
		ExpectedURLParams: map[string]string{"msg": `Error Message`, "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "3F1E492B2186551570F24C2F07D5D7E2"},
		ExpectedHeaders:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:          setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+252788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus: "W",
		MockResponseBody:  `<return>Success</return>`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"msg": "My pic!\nhttps://foo.bar/image.jpg", "to": "0788383383", "from": "2020", "username": "Username", "password": "Password", "key": "DBE569579FD899628C17254ECCE15DB7"},
		ExpectedHeaders:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		SendPrep:          setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TS", "2020", "US",
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
