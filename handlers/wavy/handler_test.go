package wavy

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR", nil),
}

var (
	receiveURL         = "/c/wv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	sentStatusURL      = "/c/wv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/sent/"
	deliveredStatusURL = "/c/wv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/delivered/"

	validReceive = `{
		"id": "external_id",
		"subAccount": "iFoodMarketing",
		"campaignAlias": "iFoodPromo",
		"carrierId": 1,
		"carrierName": "VIVO",
		"source": "5516981562820",
		"shortCode": "2020",
		"messageText": "Eu quero pizza",
		"receivedAt": 1459991487970,
		"receivedDate": "2016-09-05T12:13:25Z",
		"mt": {
			"id": "8be584fd-2554-439b-9ba9-aab507278992",
			"correlationId": "1876",
			"username": "iFoodCS",
			"email": "customer.support@ifood.com"
		}
	}`

	validSentStatus = `{
		"id": "58b36497-fb0f-474c-9c35-20b184ac4227",
		"correlationId": "12345",
		"sentStatusCode": 2,
		"sentStatus": "SENT_SUCCESS"
	}
	`
	unknownSentStatus = `{
		"id": "58b36497-fb0f-474c-9c35-20b184ac4227",
		"correlationId": "12345",
		"sentStatusCode": 777,
		"sentStatus": "Blabla"
	}
	`

	validDeliveredStatus = `{
		"id": "58b36497-fb0f-474c-9c35-20b184ac4227",
		"correlationId": "12345",
		"sentStatusCode ": 2,
		"sentStatus": "SENT_SUCCESS ",
		"deliveredStatusCode": 4,
		"deliveredStatus": "DELIVERED_SUCCESS"
	}
	`

	unknownDeliveredStatus = `{
		"id": "58b36497-fb0f-474c-9c35-20b184ac4227",
		"correlationId": "12345",
		"sentStatusCode ": 2,
		"sentStatus": "SENT_SUCCESS",
		"deliveredStatusCode": 777,
		"deliveredStatus": "BlaBal"
	}
	`
)

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Message",
		URL:                  receiveURL,
		Data:                 validReceive,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Message Accepted",
		ExpectedMsgText:      Sp("Eu quero pizza"),
		ExpectedURN:          "tel:+5516981562820",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
	},
	{
		Label:                "Invalid JSON receive",
		URL:                  receiveURL,
		Data:                 `blargh`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request JSON",
	},
	{
		Label:                "Missing Keys receive",
		URL:                  receiveURL,
		Data:                 `{}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'ID' failed on the 'required'",
	},
	{
		Label:                "Sent Status Valid",
		URL:                  sentStatusURL,
		Data:                 validSentStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Status Update Accepted",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusSent}},
	},
	{
		Label:                "Unknown Sent Status Valid",
		URL:                  sentStatusURL,
		Data:                 unknownSentStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown sent status code",
	},
	{
		Label:                "Invalid JSON sent Status",
		URL:                  sentStatusURL,
		Data:                 `blargh`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request JSON",
	},
	{
		Label:                "Missing Keys sent Status",
		URL:                  sentStatusURL,
		Data:                 `{}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'CollerationID' failed on the 'required'",
	},
	{
		Label:                "Delivered Status Valid",
		URL:                  deliveredStatusURL,
		Data:                 validDeliveredStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Status Update Accepted",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "12345", Status: courier.MsgStatusDelivered}},
	},
	{
		Label:                "Unknown Delivered Status Valid",
		URL:                  deliveredStatusURL,
		Data:                 unknownDeliveredStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unknown delivered status code",
	},
	{
		Label:                "Invalid JSON delivered Statu",
		URL:                  deliveredStatusURL,
		Data:                 `blargh`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse request JSON",
	},
	{
		Label:                "Missing Keys sent Status",
		URL:                  deliveredStatusURL,
		Data:                 `{}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "validation for 'CollerationID' failed on the 'required'",
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
		Label:               "Plain Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{"id": "external1"}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"username": "user1", "authenticationtoken": "token", "Accept": "application/json", "Content-Type": "application/json"},
		ExpectedRequestBody: `{"destination":"250788383383","messageText":"Simple Message ☺\nhttps://foo.bar/image.jpg"}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "external1",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error status 403",
		MsgText:             "Error Response",
		MsgURN:              "tel:+250788383383",
		MockResponseStatus:  403,
		ExpectedMsgStatus:   "E",
		ExpectedRequestBody: `{"destination":"250788383383","messageText":"Error Response"}`,
		SendPrep:            setSendURL,
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
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR",
		map[string]any{
			courier.ConfigUsername:  "user1",
			courier.ConfigAuthToken: "token",
		})
	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"token"}, nil)
}
