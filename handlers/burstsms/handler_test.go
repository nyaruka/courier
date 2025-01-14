package burstsms

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
	receiveURL = "/c/bs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/bs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL + "?response=Msg&mobile=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
	},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL + "?response=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'mobile'",
	},
	{
		Label:                "Status Valid",
		URL:                  statusURL + "?message_id=12345&status=pending",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Status Update Accepted",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  statusURL + "?message_id=12345&status=unknown",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status value",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BS", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTestCases(t, chs, newHandler(), testCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:          "Plain Send",
		MsgText:        "Simple Message ☺",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.transmitsms.com/send-sms.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "message_id": 19835, "recipients": 3, "cost": 1.000 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"to":      {"250788383383"},
					"message": {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"from":    {"2020"},
				},
			},
		},
		ExpectedExtIDs: []string{"19835"},
	},
	{
		Label:   "Invalid JSON",
		MsgText: "Invalid JSON",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.transmitsms.com/send-sms.json": {
				httpx.NewMockResponse(200, nil, []byte(`not json`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"to":      {"250788383383"},
					"message": {"Invalid JSON"},
					"from":    {"2020"},
				},
			},
		},
		ExpectedError: courier.ErrResponseUnparseable,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Response",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.transmitsms.com/send-sms.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "message_id": 0 }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"to":      {"250788383383"},
					"message": {"Error Response"},
					"from":    {"2020"},
				},
			},
		},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.transmitsms.com/send-sms.json": {
				httpx.NewMockResponse(501, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"to":      {"250788383383"},
					"message": {"Error Message"},
					"from":    {"2020"},
				},
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BS", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{courier.ConfigUsername: "user1", courier.ConfigPassword: "pass1"},
	)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{httpx.BasicAuth("user1", "pass1")}, nil)
}
