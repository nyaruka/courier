package arabiacell

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
	receiveURL = "/c/ac/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "B=Msg&M=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
	},
	{
		Label:                "Receive Missing Number",
		URL:                  receiveURL,
		Data:                 "B=Msg",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "required field 'M'",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US", []string{urns.Phone.Prefix}, nil),
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
			"https://acsdp.arabiacell.net": {
				httpx.NewMockResponse(200, nil, []byte(`<response><code>204</code><text>MT is successfully sent</text><message_id>external1</message_id></response>`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"userName":      {"user1"},
					"password":      {"pass1"},
					"handlerType":   {"send_msg"},
					"serviceId":     {"service1"},
					"msisdn":        {"+250788383383"},
					"messageBody":   {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"chargingLevel": {"0"},
				},
			},
		},
		ExpectedExtIDs: []string{"external1"},
	},
	{
		Label:   "Invalid XML",
		MsgText: "Invalid XML",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://acsdp.arabiacell.net": {
				httpx.NewMockResponse(200, nil, []byte(`not xml`)),
			},
		},
		ExpectedError: courier.ErrResponseUnparseable,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Response",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://acsdp.arabiacell.net": {
				httpx.NewMockResponse(200, nil, []byte(`<response><code>501</code><text>failure</text><message_id></message_id></response>`)),
			},
		},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://acsdp.arabiacell.net": {
				httpx.NewMockResponse(501, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
			configServiceID:        "service1",
			configChargingLevel:    "0",
		})

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"pass1"}, nil)
}
