package jasmin

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

const (
	receiveURL = "/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JS", "2020", "US", []string{urns.Phone.Prefix}, nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 "content=%05v%05nement&coding=0&From=2349067554729&To=2349067554711&id=1001",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ACK/Jasmin",
		ExpectedMsgText:      Sp("événement"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "1001",
	},
	{
		Label:                "Receive Missing To",
		URL:                  receiveURL,
		Data:                 "content=%05v%05nement&coding=0&From=2349067554729&id=1001",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "field 'to' required",
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 "content=%05v%05nement&coding=0&From=MTN&To=2349067554711&id=1001",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "not a possible number",
	},
	{
		Label:                "Status Delivered",
		URL:                  statusURL,
		Data:                 "id=external1&dlvrd=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ACK/Jasmin",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "external1", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Status Failed",
		URL:                  statusURL,
		Data:                 "id=external1&err=1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ACK/Jasmin",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "external1", Status: courier.MsgStatusFailed}},
	},
	{
		Label:                "Status Missing",
		URL:                  statusURL,
		ExpectedRespStatus:   400,
		Data:                 "nothing",
		ExpectedBodyContains: "field 'id' required",
	},
	{
		Label:                "Status Unknown",
		URL:                  statusURL,
		ExpectedRespStatus:   400,
		Data:                 "id=external1&err=0&dlvrd=0",
		ExpectedBodyContains: "must have either dlvrd or err set to 1",
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
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`Success "External ID1"`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"content":    {"Simple Message"},
				"to":         {"250788383383"},
				"from":       {"2020"},
				"coding":     {"0"},
				"dlr-level":  {"2"},
				"dlr":        {"yes"},
				"dlr-method": {http.MethodPost},
				"username":   {"Username"},
				"password":   {"Password"},
				"dlr-url":    {"https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
			},
		}},
		ExpectedExtIDs: []string{"External ID1"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`Success "External ID1"`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"content":    {"?"},
				"to":         {"250788383383"},
				"from":       {"2020"},
				"coding":     {"0"},
				"dlr-level":  {"2"},
				"dlr":        {"yes"},
				"dlr-method": {http.MethodPost},
				"username":   {"Username"},
				"password":   {"Password"},
				"dlr-url":    {"https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
			},
		}},
		ExpectedExtIDs: []string{"External ID1"},
	},
	{
		Label:   "Smart Encoding",
		MsgText: "Fancy “Smart” Quotes",
		MsgURN:  "tel:+250788383383",
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`Success "External ID1"`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"content":    {`Fancy "Smart" Quotes`},
				"to":         {"250788383383"},
				"from":       {"2020"},
				"coding":     {"0"},
				"dlr-level":  {"2"},
				"dlr":        {"yes"},
				"dlr-method": {http.MethodPost},
				"username":   {"Username"},
				"password":   {"Password"},
				"dlr-url":    {"https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
			},
		}},
		ExpectedExtIDs: []string{"External ID1"},
	},
	{
		Label:           "Send Attachment",
		MsgText:         "My pic!",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: true,
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(200, nil, []byte(`Success "External ID1"`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"content":    {"My pic!\nhttps://foo.bar/image.jpg"},
				"to":         {"250788383383"},
				"from":       {"2020"},
				"coding":     {"0"},
				"dlr-level":  {"2"},
				"dlr":        {"yes"},
				"dlr-method": {http.MethodPost},
				"username":   {"Username"},
				"password":   {"Password"},
				"dlr-url":    {"https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
			},
		}},
		ExpectedExtIDs: []string{"External ID1"},
	},
	{
		Label:           "Error Sending",
		MsgText:         "Error Message",
		MsgURN:          "tel:+250788383383",
		MsgHighPriority: false,
		MockResponses: map[string][]*httpx.MockResponse{
			"http://example.com/send*": {
				httpx.NewMockResponse(401, nil, []byte(`Failed Sending`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Params: url.Values{
				"content":    {"Error Message"},
				"to":         {"250788383383"},
				"from":       {"2020"},
				"coding":     {"0"},
				"dlr-level":  {"2"},
				"dlr":        {"yes"},
				"dlr-method": {http.MethodPost},
				"username":   {"Username"},
				"password":   {"Password"},
				"dlr-url":    {"https://localhost/c/js/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"},
			},
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JS", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":            "Password",
			"username":            "Username",
			courier.ConfigSendURL: "http://example.com/send",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"Password"}, nil)
}
