package m3tech

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US", nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?from=+923161909799&text=hello+world",
		Data:                 " ",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "SMS Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+923161909799",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?from=MTN&text=hello+world",
		Data:                 " ",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Receive No From",
		URL:                  "/c/m3/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?text=hello",
		Data:                 " ",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing required field 'from'",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"Response": "0"}]`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{
			"MobileNo":    "250788383383",
			"SMS":         "Simple Message",
			"SMSChannel":  "0",
			"AuthKey":     "m3-Tech",
			"HandsetPort": "0",
			"MsgHeader":   "2020",
			"Telco":       "0",
			"SMSType":     "0",
			"UserId":      "Username",
			"Password":    "Password",
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"Response": "0"}]`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"SMS": "☺", "SMSType": "7"},
		SendPrep:          setSendURL},
	{Label: "Smart Encoding",
		MsgText: "Fancy “Smart” Quotes", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"Response": "0"}]`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"SMS": `Fancy "Smart" Quotes`, "SMSType": "0"},
		SendPrep:          setSendURL},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus: "W",
		MockResponseBody:  `[{"Response": "0"}]`, MockResponseStatus: 200,
		ExpectedURLParams: map[string]string{"SMS": "My pic!\nhttps://foo.bar/image.jpg", "SMSType": "0"},
		SendPrep:          setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Sending", MsgURN: "tel:+250788383383",
		ExpectedMsgStatus: "E",
		MockResponseBody:  `[{"Response": "101"}]`, MockResponseStatus: 403,
		SendPrep: setSendURL},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US",
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
