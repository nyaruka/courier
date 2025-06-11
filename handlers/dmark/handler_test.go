package dmark

import (
	"net/url"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DM", "2020", "RW", []string{urns.Phone.Prefix}, nil),
}

const (
	receiveURL = "/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "text=Msg&short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=254791541111",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Msg"),
		ExpectedURN:          "tel:+254791541111",
		ExpectedDate:         time.Date(2017, 10, 26, 15, 51, 32, 906335000, time.UTC),
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 "text=Msg&short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=MTN",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Receive Empty",
		URL:                  receiveURL,
		Data:                 "empty",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'msisdn' required",
	},
	{
		Label:                "Receive Missing Text",
		URL:                  receiveURL,
		Data:                 "short_code=2020&tstamp=2017-10-26T15:51:32.906335%2B00:00&msisdn=254791541111",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'text' required",
	},
	{
		Label:                "Receive Invalid TS",
		URL:                  receiveURL,
		Data:                 "text=Msg&short_code=2020&tstamp=2017-10-26&msisdn=254791541111",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid tstamp",
	},
	{
		Label:                "Status Invalid",
		URL:                  statusURL,
		Data:                 "id=12345&status=Borked",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown status",
	},
	{
		Label:                "Status Missing",
		URL:                  statusURL,
		Data:                 "id=12345",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'status' required",
	},
	{
		Label:                "Status Valid",
		URL:                  statusURL,
		Data:                 "id=12345&status=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi1.dmarkmobile.com/sms/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "type": "MT", "sms_id": "6b1c15d3-cba2-46f7-9a25-78265e58057d" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Authorization": "Token Authy"},
			Form: url.Values{
				"text":     {"Simple Message ☺"},
				"receiver": {"250788383383"},
				"sender":   {"2020"},
				"dlr_url":  {"https://localhost/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%s"},
			},
		}},
		ExpectedExtIDs: []string{"6b1c15d3-cba2-46f7-9a25-78265e58057d"},
	},
	{
		Label:   "Invalid Body",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi1.dmarkmobile.com/sms/": {
				httpx.NewMockResponse(200, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Authorization": "Token Authy"},
			Form: url.Values{
				"text":     {"Error Message"},
				"receiver": {"250788383383"},
				"sender":   {"2020"},
				"dlr_url":  {"https://localhost/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%s"},
			},
		}},
		ExpectedError: courier.ErrResponseContent,
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://smsapi1.dmarkmobile.com/sms/": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Authorization": "Token Authy"},
			Form: url.Values{
				"text":     {"Error Message"},
				"receiver": {"250788383383"},
				"sender":   {"2020"},
				"dlr_url":  {"https://localhost/c/dk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?id=10&status=%s"},
			},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			courier.ConfigAuthToken: "Authy",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Authy"}, nil)
}
