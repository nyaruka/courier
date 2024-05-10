package m3tech

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US", []string{urns.Phone.Prefix}, nil),
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
		ExpectedBodyContains: "not a possible number",
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

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`[{"Response": "0"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"MobileNo":    {"250788383383"},
				"SMS":         {"Simple Message"},
				"SMSChannel":  {"0"},
				"AuthKey":     {"m3-Tech"},
				"HandsetPort": {"0"},
				"MsgHeader":   {"2020"},
				"MsgId":       {"10"},
				"Telco":       {"0"},
				"SMSType":     {"0"},
				"UserId":      {"Username"},
				"Password":    {"Password"},
			},
		}},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`[{"Response": "0"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"SMS":         {"☺"},
				"MobileNo":    {"250788383383"},
				"SMSChannel":  {"0"},
				"AuthKey":     {"m3-Tech"},
				"HandsetPort": {"0"},
				"MsgHeader":   {"2020"},
				"MsgId":       {"10"},
				"Telco":       {"0"},
				"SMSType":     {"7"},
				"UserId":      {"Username"},
				"Password":    {"Password"},
			},
		}},
	},
	{
		Label:   "Smart Encoding",
		MsgText: "Fancy “Smart” Quotes",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`[{"Response": "0"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"SMS":         {`Fancy "Smart" Quotes`},
				"MobileNo":    {"250788383383"},
				"SMSChannel":  {"0"},
				"AuthKey":     {"m3-Tech"},
				"HandsetPort": {"0"},
				"MsgHeader":   {"2020"},
				"MsgId":       {"10"},
				"Telco":       {"0"},
				"SMSType":     {"0"},
				"UserId":      {"Username"},
				"Password":    {"Password"},
			},
		}},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+250788383383",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS*": {
				httpx.NewMockResponse(200, nil, []byte(`[{"Response": "0"}]`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"SMS":         {"My pic!\nhttps://foo.bar/image.jpg"},
				"MobileNo":    {"250788383383"},
				"SMSChannel":  {"0"},
				"AuthKey":     {"m3-Tech"},
				"HandsetPort": {"0"},
				"MsgHeader":   {"2020"},
				"MsgId":       {"10"},
				"Telco":       {"0"},
				"SMSType":     {"0"},
				"UserId":      {"Username"},
				"Password":    {"Password"},
			},
		}},
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Sending",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://secure.m3techservice.com/GenericServiceRestAPI/api/SendSMS*": {
				httpx.NewMockResponse(403, nil, []byte(`[{"Response": "101"}]`)),
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "M3", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password": "Password",
			"username": "Username",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
