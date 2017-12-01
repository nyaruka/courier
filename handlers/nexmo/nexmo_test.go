package nexmo

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US", nil),
}

var (
	statusURL  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status"
	receiveURL = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"

	receiveValidMessage = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive?to=2020&msisdn=2349067554729&text=Join&messageId=external1"

	statusDelivered  = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=delivered"
	statusExpired    = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=expired"
	statusFailed     = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=failed"
	statusAccepted   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=accepted"
	statusBuffered   = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=buffered"
	statusUnexpected = "/c/nx/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status?to=2020&messageId=external1&status=unexpected"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Valid Receive", URL: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Join"), URN: Sp("tel:+2349067554729")},
	{Label: "Receive URL check", URL: receiveURL, Status: 200, Response: "no to parameter, ignored"},
	{Label: "Status URL check", URL: statusURL, Status: 200, Response: "no messageId parameter, ignored"},

	{Label: "Status delivered", URL: statusDelivered, Status: 200, Response: `"status":"D"`, ExternalID: Sp("external1")},
	{Label: "Status expired", URL: statusExpired, Status: 200, Response: `"status":"F"`, ExternalID: Sp("external1")},
	{Label: "Status failed", URL: statusFailed, Status: 200, Response: `"status":"F"`, ExternalID: Sp("external1")},
	{Label: "Status accepted", URL: statusAccepted, Status: 200, Response: `"status":"S"`, ExternalID: Sp("external1")},
	{Label: "Status buffered", URL: statusBuffered, Status: 200, Response: `"status":"S"`, ExternalID: Sp("external1")},
	{Label: "Status unexpected", URL: statusUnexpected, Status: 200, Response: "ignoring unknown status report", ExternalID: Sp("external1")},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ResponseBody: `{"messages":[{"status":"0","message-id":"1002"}]}`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "Unicode ☺", URN: "tel:+250788383383",
		Status: "W", ExternalID: "1002",
		URLParams:    map[string]string{"text": "Unicode ☺", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "unicode"},
		ResponseBody: `{"messages":[{"status":"0","message-id":"1002"}]}`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1002",
		URLParams:    map[string]string{"text": "My pic!\nhttps://foo.bar/image.jpg", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ResponseBody: `{"messages":[{"status":"0","message-id":"1002"}]}`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Error Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ResponseBody: `Error`, ResponseStatus: 400,
		SendPrep: setSendURL},
	{Label: "Invalid Token",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "E",
		URLParams:    map[string]string{"text": "Simple Message", "to": "250788383383", "from": "2020", "api_key": "nexmo-api-key", "api_secret": "nexmo-api-secret", "status-report-req": "1", "type": "text"},
		ResponseBody: "Invalid API token", ResponseStatus: 401,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "NX", "2020", "US",
		map[string]interface{}{
			configNexmoAPIKey:        "nexmo-api-key",
			configNexmoAPISecret:     "nexmo-api-secret",
			configNexmoAppID:         "nexmo-app-id",
			configNexmoAppPrivateKey: "nexmo-app-private-key",
		})

	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
