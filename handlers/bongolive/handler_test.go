package bongolive

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
	receiveURL = "/c/bl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var incomingCases = []IncomingTestCase{
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
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE", []string{urns.Phone.Prefix}, nil),
	}
	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:          "Plain Send",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.blsmsgw.com:8443/bin/send.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results": [{"status": "0", "msgid": "123"}]}`)),
			},
		},
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
		ExpectedExtIDs: []string{"123"},
	},
	{
		Label:          "Bad Status",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.blsmsgw.com:8443/bin/send.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results": [{"status": "3"}]}`)),
			},
		},
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
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error status 403",
		MsgText: "Error Response",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.blsmsgw.com:8443/bin/send.json*": {
				httpx.NewMockResponse(403, nil, []byte(`{"results": [{"status": "1", "msgid": "123"}]}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{
					"USERNAME":   {"user1"},
					"PASSWORD":   {"pass1"},
					"SOURCEADDR": {"2020"},
					"DESTADDR":   {"250788383383"},
					"DLR":        {"1"},
					"MESSAGE":    {"Error Response"},
				},
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.blsmsgw.com:8443/bin/send.json*": {
				httpx.NewMockResponse(501, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Params: url.Values{
					"USERNAME":   {"user1"},
					"PASSWORD":   {"pass1"},
					"SOURCEADDR": {"2020"},
					"DESTADDR":   {"250788383383"},
					"DLR":        {"1"},
					"MESSAGE":    {"Error Message"},
				},
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BL", "2020", "KE",
		[]string{urns.Phone.Prefix},
		map[string]any{courier.ConfigUsername: "user1", courier.ConfigPassword: "pass1"},
	)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"pass1"}, nil)
}
