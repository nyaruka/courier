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

var daTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL + "?userid=testusr&password=test&original=6289881134560&sendto=2020&message=Msg&messageid=foo",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "000",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+6289881134560",
		ExpectedExternalID:   "foo",
	},
	{
		Label:                "Receive Valid",
		URL:                  receiveURL + "?userid=testusr&password=test&original=cmp-oodddqddwdwdcd&sendto=2020&message=Msg&messageid=bar",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "000",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "ext:cmp-oodddqddwdwdcd",
		ExpectedExternalID:   "bar",
	},
	{
		Label:                "Receive Invalid",
		URL:                  receiveURL,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing required parameters original and sendto",
	},

	{
		Label:                "Valid Status",
		URL:                  statusURL + "?status=10&messageid=12345",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "000",
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Valid Status",
		URL:                  statusURL + "?status=10&messageid=12345.2",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "000",
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Failed Status",
		URL:                  statusURL + "?status=30&messageid=12345",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "000",
		ExpectedStatuses:     []ExpectedStatus{{MsgID: 12345, Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Missing Status",
		URL:                  statusURL + "?messageid=12345",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "parameters messageid and status should not be empty",
	},
	{
		Label:                "Missing Status",
		URL:                  statusURL + "?status=foo&messageid=12345",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "parsing failed: status 'foo' is not an integer",
	},
	{
		Label:                "Missing Status",
		URL:                  statusURL + "?status=10&messageid=abc",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "parsing failed: messageid 'abc' is not an integer",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, daTestChannels, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), daTestCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	daHandler := h.(*handler)
	daHandler.sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
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
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("001", "Authentication error.")},
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
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("101", "Account expired or invalid parameters.")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultDAChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID",
		map[string]any{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
		})

	RunOutgoingTestCases(t, defaultDAChannel, NewHandler("DA", "Dartmedia", sendURL, maxMsgLength), defaultSendTestCases, []string{"Password"}, nil)
}
