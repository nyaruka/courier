package mtarget

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var (
	receiveURL = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = "Msisdn=+923161909799&Content=hello+world&Keyword=Default&MsgId=foo"
	receiveInvalidURN   = "Msisdn=MTN&Content=hello+world&Keyword=Default"
	receiveStop         = "Msisdn=+923161909799&Content=Stop&Keyword=Stop"
	receiveMissingFrom  = "Content=hello&Keyword=Default"

	receivePart2 = "Msisdn=+923161909799&Content=world&Keyword=Default&msglong.id=longmsg&msglong.msgcount=2&msglong.msgref=2"
	receivePart1 = "Msisdn=+923161909799&Content=hello+&Keyword=Default&msglong.id=longmsg&msglong.msgcount=2&msglong.msgref=1"

	statusURL = "/c/mt/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"

	statusDelivered = "MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=3"
	statusFailed    = "MsgId=12a7ee90-50ce-11e7-80ae-00000a0a643c&Status=4"
	statusMissingID = "status?Status=4"
)

var incomingCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 receiveValidMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+923161909799",
		ExpectedExternalID:   "foo",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 receiveInvalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive Stop",
		URL:                  receiveURL,
		Data:                 receiveStop,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeStopContact, URN: "tel:+923161909799"},
		},
	},
	{
		Label:                "Receive Missing From",
		URL:                  receiveURL,
		Data:                 receiveMissingFrom,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing required field 'Msisdn'",
	},
	{
		Label:                "Receive Part 2",
		URL:                  receiveURL,
		Data:                 receivePart2,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "received",
	},
	{
		Label:                "Receive Part 1",
		URL:                  receiveURL,
		Data:                 receivePart1,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+923161909799",
	},
	{
		Label:                "Status Delivered",
		URL:                  statusURL,
		Data:                 statusDelivered,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12a7ee90-50ce-11e7-80ae-00000a0a643c", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Status Failed",
		URL:                  statusURL,
		Data:                 statusFailed,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12a7ee90-50ce-11e7-80ae-00000a0a643c", Status: courier.MsgStatusFailed},
		},
	},
	{
		Label:                "Status Missing ID",
		URL:                  statusURL,
		Data:                 statusMissingID,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "missing required field 'MsgId'",
	},
}

func TestIncoming(t *testing.T) {
	chs := []courier.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTestCases(t, chs, newHandler(), incomingCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api-public.mtarget.fr/api-sms.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results":[{"code": "0", "ticket": "externalID"}]}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"msisdn": {"+250788383383"}, "msg": {"Simple Message"}, "username": {"Username"}, "password": {"Password"}, "serviceid": {"2020"}, "allowunicode": {"true"}}},
		},
		ExpectedExtIDs: []string{"externalID"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api-public.mtarget.fr/api-sms.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results":[{"code": "0", "ticket": "externalID"}]}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"msisdn": {"+250788383383"}, "msg": {"☺"}, "username": {"Username"}, "password": {"Password"}, "serviceid": {"2020"}, "allowunicode": {"true"}}},
		},
		ExpectedExtIDs: []string{"externalID"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api-public.mtarget.fr/api-sms.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results":[{"code": "0", "ticket": "externalID"}]}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Params: url.Values{"msisdn": {"+250788383383"}, "msg": {"My pic!\nhttps://foo.bar/image.jpg"}, "username": {"Username"}, "password": {"Password"}, "serviceid": {"2020"}, "allowunicode": {"true"}}},
		},
		ExpectedExtIDs: []string{"externalID"},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Sending",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api-public.mtarget.fr/api-sms.json*": {
				httpx.NewMockResponse(403, nil, []byte(`{"results":[{"code": "3", "reason": "FAILED", "ticket": "null"}]}`)),
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Error Response",
		MsgText: "Error Sending",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api-public.mtarget.fr/api-sms.json*": {
				httpx.NewMockResponse(200, nil, []byte(`{"results":[{"code": "3", "reason": "FAILED", "ticket": "null"}]}`)),
			},
		},
		ExpectedError: courier.ErrFailedWithReason("3", "FAILED"),
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MT", "2020", "FR",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), outgoingCases, []string{"Password"}, nil)
}
