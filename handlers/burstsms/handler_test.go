package burstsms

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BS", "2020", "US", nil),
}

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
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{ "message_id": 19835, "recipients": 3, "cost": 1.000 }`,
		MockResponseStatus: 200,
		ExpectedRequests: []ExpectedRequest{
			{
				Form: url.Values{
					"to":      {"250788383383"},
					"message": {"Simple Message ☺\nhttps://foo.bar/image.jpg"},
					"from":    {"2020"},
				},
			},
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "19835",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid JSON",
		MsgText:            "Invalid JSON",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `not json`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseUnparseable("XML")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Response",
		MsgText:            "Error Response",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{ "message_id": 0 }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Bad Gateway`,
		MockResponseStatus: 501,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BS", "2020", "US",
		map[string]any{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{httpx.BasicAuth("user1", "pass1")}, nil)
}
