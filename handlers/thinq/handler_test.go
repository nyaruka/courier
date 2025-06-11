package thinq

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US", []string{urns.Phone.Prefix}, nil),
}

const (
	receiveURL = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testJpgBase64 = base64.StdEncoding.EncodeToString(test.ReadFile("../../test/testdata/test.jpg"))

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Valid",
		URL:                  receiveURL,
		Data:                 "message=hello+world&from=2065551234&type=sms&to=2065551212",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+12065551234",
	},
	{
		Label:                "Receive No Params",
		URL:                  receiveURL,
		Data:                 " ",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `'From' failed on the 'required'`,
	},
	{
		Label:                "Receive attachment as URL",
		URL:                  receiveURL,
		Data:                 "message=http://foo.bar/foo.png&from=2065551234&type=mms&to=2065551212",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedURN:          "tel:+12065551234",
		ExpectedAttachments:  []string{"http://foo.bar/foo.png"},
	},
	{
		Label:                "Receive attachment as base64",
		URL:                  receiveURL,
		Data:                 fmt.Sprintf("message=%s&from=2065551234&type=mms&to=2065551212", url.QueryEscape(testJpgBase64)),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedURN:          "tel:+12065551234",
		ExpectedAttachments:  []string{"data:" + testJpgBase64},
	},
	{
		Label:                "Status Valid",
		URL:                  statusURL,
		Data:                 "guid=1234&status=DELIVRD",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "1234", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Status Invalid",
		URL:                  statusURL,
		Data:                 "guid=1234&status=UN",
		ExpectedRespStatus:   400,
		ExpectedExternalID:   "1234",
		ExpectedBodyContains: `"unknown status: 'UN'"`,
	},
	{
		Label:                "Status Missing GUID",
		URL:                  statusURL,
		Data:                 "status=DELIVRD",
		ExpectedRespStatus:   400,
		ExpectedExternalID:   "1234",
		ExpectedBodyContains: `'GUID' failed on the 'required' tag`,
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{ "guid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Authorization": "Basic dXNlcjE6c2VzYW1l"},
			Body:    `{"from_did":"2065551212","to_did":"2067791234","message":"Simple Message ☺"}`,
		}},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:          "Send Attachment",
		MsgText:        "My pic!",
		MsgURN:         "tel:+12067791234",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{ "guid": "1002" }`)),
			},
			"https://api.thinq.com/account/1234/product/origination/mms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{ "guid": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{"from_did": {"2065551212"}, "to_did": {"2067791234"}, "media_url": {"https://foo.bar/image.jpg"}},
			},
			{
				Body: `{"from_did":"2065551212","to_did":"2067791234","message":"My pic!"}`,
			},
		},
		ExpectedExtIDs: []string{"1002", "1002"},
	},
	{
		Label:          "Only Attachment",
		MsgText:        "",
		MsgURN:         "tel:+12067791234",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/mms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{ "guid": "1002" }`)),
			},
		},
		ExpectedExtIDs: []string{"1002"},
	},
	{
		Label:   "No External ID",
		MsgText: "No External ID",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"from_did":"2065551212","to_did":"2067791234","message":"No External ID"}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("guid")},
		ExpectedError:     courier.ErrResponseContent,
	},
	{
		Label:   "Error Sending",
		MsgText: "Error Message",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(401, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"from_did":"2065551212","to_did":"2067791234","message":"Error Message"}`,
		}},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Connection Error",
		MsgText: "Error Message",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(500, nil, []byte(`{ "error": "failed" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"from_did":"2065551212","to_did":"2067791234","message":"Error Message"}`,
		}},
		ExpectedError: courier.ErrConnectionFailed,
	},
	{
		Label:   "Reponse Unexpected",
		MsgText: "Simple Message ☺",
		MsgURN:  "tel:+12067791234",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.thinq.com/account/1234/product/origination/sms/send": {
				httpx.NewMockResponse(200, nil, []byte(`{ "missing": "1002" }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Headers: map[string]string{"Authorization": "Basic dXNlcjE6c2VzYW1l"},
			Body:    `{"from_did":"2065551212","to_did":"2067791234","message":"Simple Message ☺"}`,
		}},
		ExpectedLogErrors: []*clogs.Error{courier.ErrorResponseValueMissing("guid")},
		ExpectedError:     courier.ErrResponseContent,
	},
}

func TestOutgoing(t *testing.T) {
	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			configAccountID:    "1234",
			configAPITokenUser: "user1",
			configAPIToken:     "sesame",
		})
	RunOutgoingTestCases(t, channel, newHandler(), sendTestCases, []string{httpx.BasicAuth("user1", "sesame")}, nil)
}
