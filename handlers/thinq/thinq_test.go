package thinq

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US", nil),
}

const (
	receiveURL = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var testCases = []ChannelHandleTestCase{
	{
		Label:            "Receive Valid",
		URL:              receiveURL,
		Data:             "message=hello+world&from=2065551234&type=sms&to=2065551212",
		ExpectedStatus:   200,
		ExpectedResponse: "Accepted",
		ExpectedMsgText:  Sp("hello world"),
		ExpectedURN:      "tel:+12065551234",
	},
	{
		Label:            "Receive No Params",
		URL:              receiveURL,
		Data:             " ",
		ExpectedStatus:   400,
		ExpectedResponse: `'From' failed on the 'required'`,
	},
	{
		Label:               "Receive Media",
		URL:                 receiveURL,
		Data:                "message=http://foo.bar/foo.png&hello+world&from=2065551234&type=mms&to=2065551212",
		ExpectedStatus:      200,
		ExpectedResponse:    "Accepted",
		ExpectedURN:         "tel:+12065551234",
		ExpectedAttachments: []string{"http://foo.bar/foo.png"},
	},

	{
		Label:              "Status Valid",
		URL:                statusURL,
		Data:               "guid=1234&status=DELIVRD",
		ExpectedStatus:     200,
		ExpectedExternalID: "1234",
		ExpectedResponse:   `"status":"D"`,
	},
	{
		Label:              "Status Invalid",
		URL:                statusURL,
		Data:               "guid=1234&status=UN",
		ExpectedStatus:     400,
		ExpectedExternalID: "1234",
		ExpectedResponse:   `"unknown status: 'UN'"`,
	},
	{
		Label:              "Status Missing GUID",
		URL:                statusURL,
		Data:               "status=DELIVRD",
		ExpectedStatus:     400,
		ExpectedExternalID: "1234",
		ExpectedResponse:   `'GUID' failed on the 'required' tag`,
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "?account_id=%s"
	sendMMSURL = s.URL + "?account_id=%s"
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:+12067791234",
		MockResponseBody:    `{ "guid": "1002" }`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Authorization": "Basic dXNlcjE6c2VzYW1l"},
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Simple Message ☺"}`,
		ExpectedStatus:      "W",
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
		ExpectedStatus:      "W",
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
		ExpectedStatus:     "W",
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
		ExpectedStatus:      "E",
		ExpectedErrors:      []courier.ChannelError{courier.NewChannelError("Unable to read external ID from guid field", "")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+12067791234",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Error Message"}`,
		ExpectedStatus:      "E",
		SendPrep:            setSendURL,
	},
}

func TestSending(t *testing.T) {
	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US",
		map[string]interface{}{
			configAccountID:    "1234",
			configAPITokenUser: "user1",
			configAPIToken:     "sesame",
		})
	RunChannelSendTestCases(t, channel, newHandler(), sendTestCases, nil)
}
