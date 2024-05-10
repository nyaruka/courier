package dart

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var daTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID", []string{urns.Phone.Prefix}, nil),
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

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(200, nil, []byte(`000`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"Simple Message"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
		},
	},
	{
		Label:   "Long Send",
		MsgText: "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(200, nil, []byte(`000`)),
				httpx.NewMockResponse(200, nil, []byte(`000`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
			{Params: url.Values{"message": {"I need to keep adding more things to make it work"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10.2"}}},
		},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(200, nil, []byte(`000`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"My pic!\nhttps://foo.bar/image.jpg"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
		},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(400, nil, []byte(`Error`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"Error Message"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Authentication Error",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(200, nil, []byte(`001`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"Simple Message"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
		},
		ExpectedError: courier.ErrFailedWithReason("001", "Authentication error."),
	},
	{
		Label:   "Account Expired",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://202.43.169.11/APIhttpU/receive2waysms.php*": {
				httpx.NewMockResponse(200, nil, []byte(`101`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"message": {"Simple Message"}, "sendto": {"250788383383"}, "original": {"2020"}, "userid": {"Username"}, "password": {"Password"}, "dcs": {"0"}, "udhl": {"0"}, "messageid": {"10"}}},
		},
		ExpectedError: courier.ErrFailedWithReason("101", "Account expired or invalid parameters."),
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultDAChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigUsername: "Username",
			courier.ConfigPassword: "Password",
		})

	RunOutgoingTestCases(t, defaultDAChannel, NewHandler("DA", "Dartmedia", sendURL, maxMsgLength), defaultSendTestCases, []string{"Password"}, nil)
}
