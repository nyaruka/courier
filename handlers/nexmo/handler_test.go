package nexmo

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US", nil),
}

const (
	statusURL  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testCases = []IncomingTestCase{
	{
		Label:                "Valid Receive",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=2349067554729&text=Join&messageId=external1",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "external1",
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=MTN&text=Join&messageId=external1",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "phone number supplied is not a number",
	},
	{
		Label:                "Valid Receive Post",
		URL:                  receiveURL,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		Data:                 "to=2020&msisdn=2349067554729&text=Join&messageId=external1",
		ExpectedMsgText:      Sp("Join"),
		ExpectedURN:          "tel:+2349067554729",
		ExpectedExternalID:   "external1",
	},
	{
		Label:                "Receive URL check",
		URL:                  receiveURL,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no to parameter, ignored",
	},
	{
		Label:                "Status URL check",
		URL:                  statusURL,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no messageId parameter, ignored",
	},
	{
		Label:                "Status delivered",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=delivered&err-code=0",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"D"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external1", Status: courier.MsgStatusDelivered},
		},
	},
	{
		Label:                "Status expired",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=expired&err-code=0",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external1", Status: courier.MsgStatusFailed},
		},
	},
	{
		Label:                "Status failed",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=failed&err-code=6",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"F"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external1", Status: courier.MsgStatusFailed},
		},
		ExpectedErrors: []*courier.ChannelError{courier.ErrorExternal("dlr:6", "Anti-Spam Rejection")},
	},
	{
		Label:                "Status accepted",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=accepted",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external1", Status: courier.MsgStatusSent},
		},
	},
	{
		Label:                "Status buffered",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=buffered",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external1", Status: courier.MsgStatusSent},
		},
	},
	{
		Label:                "Status unexpected",
		URL:                  "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=unexpected",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring unknown status report",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"0","message-id":"1002"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "Unicode ☺",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"0","message-id":"1002"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Unicode ☺", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "unicode"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"0","message-id":"1002"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "I need to keep adding more things to make it work", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{"messages":[{"status":"0","message-id":"1002"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "1002",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Status",
		MsgText:            "Error status",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"10"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Error status", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("send:10", "Too Many Existing Binds")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Error`,
		MockResponseStatus: 400,
		ExpectedPostParams: map[string]string{"text": "Error Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid Token",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "Invalid API token",
		MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Throttled by Nexmo",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"1","error-text":"Throughput Rate Exceeded - please wait [ 250 ] and retry"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("send:1", "Throttled")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US",
		map[string]any{
			configNexmoAPIKey:        "nexmo-api-key",
			configNexmoAPISecret:     "nexmo-api-secret",
			configNexmoAppID:         "nexmo-app-id",
			configNexmoAppPrivateKey: "nexmo-app-private-key",
		})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"nexmo-api-secret", "nexmo-app-private-key"}, nil)
}
