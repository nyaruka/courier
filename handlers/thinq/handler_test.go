package thinq

import (
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US", nil),
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

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL + "?account_id=%s"
	sendMMSURL = s.URL + "?account_id=%s"
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:+12067791234",
		MockResponseBody:    `{ "guid": "1002" }`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "Basic dXNlcjE6c2VzYW1l"},
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Simple Message ☺"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "1002",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+12067791234",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{ "guid": "1002" }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"My pic!"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "1002",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Only Attachment",
		MsgText:            "",
		MsgURN:             "tel:+12067791234",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{ "guid": "1002" }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:               "No External ID",
		MsgText:             "No External ID",
		MsgURN:              "tel:+12067791234",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"No External ID"}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseValueMissing("guid")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+12067791234",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Error Message"}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US",
		map[string]any{
			configAccountID:    "1234",
			configAPITokenUser: "user1",
			configAPIToken:     "sesame",
		})
	RunOutgoingTestCases(t, channel, newHandler(), sendTestCases, []string{httpx.BasicAuth("user1", "sesame")}, nil)
}
