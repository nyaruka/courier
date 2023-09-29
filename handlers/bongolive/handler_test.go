package bongolive

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE", nil),
}

const (
	receiveURL = "/c/bl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "msgtype=1&id=12345678&message=Msg&sourceaddr=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
	},
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "id=12345678&message=Msg&sourceaddr=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
	},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Data:                 "msgtype=1&id=12345679&message=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "",
	},
	{
		Label:                "Status No params",
		URL:                  receiveURL,
		Data:                 "&",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "",
	},
	{
		Label:                "Status invalid params",
		URL:                  receiveURL,
		Data:                 "msgtype=5&dlrid=12345&status=12",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "",
	},
	{
		Label:                "Status valid",
		URL:                  receiveURL,
		Data:                 "msgtype=5&dlrid=12345&status=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Invalid Msg Type",
		URL:                  receiveURL,
		Data:                 "msgtype=3&id=12345&status=1",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"results": [{"status": "0", "msgid": "123"}]}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{
					"USERNAME":   {"user1"},
					"PASSWORD":   {"pass1"},
					"SOURCEADDR": {"2020"},
					"DESTADDR":   {"250788383383"},
					"DLR":        {"1"},
					"MESSAGE":    {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"CHARCODE":   {"2"},
				},
			},
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "123",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Bad Status",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"results": [{"status": "3"}]}`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{
					"USERNAME":   {"user1"},
					"PASSWORD":   {"pass1"},
					"SOURCEADDR": {"2020"},
					"DESTADDR":   {"250788383383"},
					"DLR":        {"1"},
					"MESSAGE":    {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"CHARCODE":   {"2"},
				},
			},
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Error status 403",
		MsgText:            "Error Response",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"results": [{"status": "1", "msgid": "123"}]}`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Bad Gateway`,
		MockResponseStatus: 501,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE",
		map[string]any{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"pass1"}, nil)
}
