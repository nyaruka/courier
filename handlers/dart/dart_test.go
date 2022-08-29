package dart

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var daTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID", nil),
}

const (
	receiveURL = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/da/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"
)

var daTestCases = []ChannelHandleTestCase{
	{
		Label:              "Receive Valid",
		URL:                receiveURL + "?userid=testusr&password=test&original=6289881134560&sendto=2020&message=Msg",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "000",
		ExpectedMsgText:    Sp("Msg"),
		ExpectedURN:        "tel:+6289881134560",
	},
	{
		Label:              "Receive Valid",
		URL:                receiveURL + "?userid=testusr&password=test&original=cmp-oodddqddwdwdcd&sendto=2020&message=Msg",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "000",
		ExpectedMsgText:    Sp("Msg"),
		ExpectedURN:        "ext:cmp-oodddqddwdwdcd",
	},
	{
		Label:              "Receive Invalid",
		URL:                receiveURL,
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "missing required parameters original and sendto",
	},

	{
		Label:              "Valid Status",
		URL:                statusURL + "?status=10&messageid=12345",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "000",
		ExpectedMsgStatus:  "D",
	},
	{
		Label:              "Valid Status",
		URL:                statusURL + "?status=10&messageid=12345.2",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "000",
		ExpectedMsgStatus:  "D",
	},
	{
		Label:              "Failed Status",
		URL:                statusURL + "?status=30&messageid=12345",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "000",
		ExpectedMsgStatus:  "F",
	},
	{
		Label:              "Missing Status",
		URL:                statusURL + "?messageid=12345",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "parameters messageid and status should not be empty",
	},
	{
		Label:              "Missing Status",
		URL:                statusURL + "?status=foo&messageid=12345",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "parsing failed: status 'foo' is not an integer",
	},
	{
		Label:              "Missing Status",
		URL:                statusURL + "?status=10&messageid=abc",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "parsing failed: messageid 'abc' is not an integer",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	daHandler := h.(*handler)
	daHandler.sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "000",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "000",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"message": "I need to keep adding more things to make it work", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10.2"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   "000",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"message": "My pic!\nhttps://foo.bar/image.jpg", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Error`,
		MockResponseStatus: 400,
		ExpectedURLParams:  map[string]string{"message": "Error Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Authentication Error",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "001",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("Error 001: Authentication Error", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Account Expired",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "101",
		MockResponseStatus: 200,
		ExpectedURLParams:  map[string]string{"message": "Simple Message", "sendto": "250788383383", "original": "2020", "userid": "Username", "password": "Password", "dcs": "0", "udhl": "0", "messageid": "10"},
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("Error 101: Account expired or invalid parameters", "")},
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultDAChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID",
		map[string]interface{}{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
		})

	RunChannelSendTestCases(t, defaultDAChannel, NewHandler("DA", "Dartmedia", sendURL, maxMsgLength), defaultSendTestCases, nil)
}
