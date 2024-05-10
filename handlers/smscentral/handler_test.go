package smscentral

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	receiveURL = "/c/sc/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{"username": "Username", "password": "Password"}),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 "mobile=%2B2349067554729&message=Join",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive No Message",
		URL:                  receiveURL,
		Data:                 "mobile=%2B2349067554729",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "tel:+2349067554729",
	},
	{
		Label:                "Receive invalid URN",
		URL:                  receiveURL,
		Data:                 "mobile=MTN&message=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive No Params",
		URL:                  receiveURL,
		Data:                 "none",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
	{
		Label:                "Receive No Sender",
		URL:                  receiveURL,
		Data:                 "message=Join",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'mobile' required",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{Label: "Plain Send",
		MsgText: "Simple Message", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://smail.smscentral.com.np/bp/ApiSms.php": {
				httpx.NewMockResponse(200, nil, []byte(`[{"id": "1002"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"content": {"Simple Message"}, "mobile": {"250788383383"}, "pass": {"Password"}, "user": {"Username"}},
		}},
	},
	{Label: "Unicode Send",
		MsgText: "☺", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://smail.smscentral.com.np/bp/ApiSms.php": {
				httpx.NewMockResponse(200, nil, []byte(`[{"id": "1002"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"content": {"☺"}, "mobile": {"250788383383"}, "pass": {"Password"}, "user": {"Username"}},
		}},
	},
	{Label: "Send Attachment",
		MsgText: "My pic!", MsgURN: "tel:+250788383383", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://smail.smscentral.com.np/bp/ApiSms.php": {
				httpx.NewMockResponse(200, nil, []byte(`[{"id": "1002"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"content": {"My pic!\nhttps://foo.bar/image.jpg"}, "mobile": {"250788383383"}, "pass": {"Password"}, "user": {"Username"}},
		}},
	},
	{Label: "Error Sending",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://smail.smscentral.com.np/bp/ApiSms.php": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"content": {`Error Message`}, "mobile": {"250788383383"}, "pass": {"Password"}, "user": {"Username"}},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{Label: "Connection Error",
		MsgText: "Error Message", MsgURN: "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://smail.smscentral.com.np/bp/ApiSms.php": {
				httpx.NewMockResponse(500, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Form: url.Values{"content": {`Error Message`}, "mobile": {"250788383383"}, "pass": {"Password"}, "user": {"Username"}},
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
