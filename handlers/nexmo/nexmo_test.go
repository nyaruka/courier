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

var (
	statusURL  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage     = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=2349067554729&text=Join&messageId=external1"
	receiveInvalidURN       = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=MTN&text=Join&messageId=external1"
	receiveValidMessageBody = "to=2020&msisdn=2349067554729&text=Join&messageId=external1"

	statusDelivered  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=delivered"
	statusExpired    = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=expired"
	statusFailed     = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=failed"
	statusAccepted   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=accepted"
	statusBuffered   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=buffered"
	statusUnexpected = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=unexpected"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Valid Receive", URL: receiveValidMessage, ExpectedStatus: 200, ExpectedResponse: "Accepted",
		ExpectedMsgText: Sp("Join"), ExpectedURN: "tel:+2349067554729"},
	{Label: "Invalid URN", URL: receiveInvalidURN, ExpectedStatus: 400, ExpectedResponse: "phone number supplied is not a number"},
	{Label: "Valid Receive Post", URL: receiveURL, ExpectedStatus: 200, ExpectedResponse: "Accepted", Data: receiveValidMessageBody,
		ExpectedMsgText: Sp("Join"), ExpectedURN: "tel:+2349067554729"},
	{Label: "Receive URL check", URL: receiveURL, ExpectedStatus: 200, ExpectedResponse: "no to parameter, ignored"},
	{Label: "Status URL check", URL: statusURL, ExpectedStatus: 200, ExpectedResponse: "no messageId parameter, ignored"},

	{Label: "Status delivered", URL: statusDelivered, ExpectedStatus: 200, ExpectedResponse: `"status":"D"`, ExpectedExternalID: "external1"},
	{Label: "Status expired", URL: statusExpired, ExpectedStatus: 200, ExpectedResponse: `"status":"F"`, ExpectedExternalID: "external1"},
	{Label: "Status failed", URL: statusFailed, ExpectedStatus: 200, ExpectedResponse: `"status":"F"`, ExpectedExternalID: "external1"},
	{Label: "Status accepted", URL: statusAccepted, ExpectedStatus: 200, ExpectedResponse: `"status":"S"`, ExpectedExternalID: "external1"},
	{Label: "Status buffered", URL: statusBuffered, ExpectedStatus: 200, ExpectedResponse: `"status":"S"`, ExpectedExternalID: "external1"},
	{Label: "Status unexpected", URL: statusUnexpected, ExpectedStatus: 200, ExpectedResponse: "ignoring unknown status report", ExpectedExternalID: "external1"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"0","message-id":"1002"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "W",
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
		ExpectedStatus:     "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("failed to send message, received error status [10]", "")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `Error`,
		MockResponseStatus: 400,
		ExpectedPostParams: map[string]string{"text": "Error Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedStatus:     "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Invalid Token",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   "Invalid API token",
		MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedStatus:     "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Throttled by Nexmo",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `{"messages":[{"status":"1","error-text":"Throughput Rate Exceeded - please wait [ 250 ] and retry"}]}`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ExpectedStatus:     "E",
		ExpectedErrors:     []courier.ChannelError{courier.NewChannelError("failed to send message, received error status [1]", "")},
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US",
		map[string]interface{}{
			configNexmoAPIKey:        "nexmo-api-key",
			configNexmoAPISecret:     "nexmo-api-secret",
			configNexmoAppID:         "nexmo-app-id",
			configNexmoAppPrivateKey: "nexmo-app-private-key",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
