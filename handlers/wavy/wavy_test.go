package wavy

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR", nil),
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

	missingRequiredKeys = `{}`

	notJSON = `blargh`

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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Eu quero pizza"), URN: Sp("tel:+5516981562820"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},
	{Label: "Invalid JSON receive", URL: receiveURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Missing Keys receive", URL: receiveURL, Data: missingRequiredKeys, Status: 400, Response: "validation for 'ID' failed on the 'required'"},

	{Label: "Sent Status Valid", URL: sentStatusURL, Data: validSentStatus, Status: 200, Response: "Status Update Accepted", MsgStatus: Sp(courier.MsgSent)},
	{Label: "Unknown Sent Status Valid", URL: sentStatusURL, Data: unknownSentStatus, Status: 400, Response: "unknown sent status code", MsgStatus: Sp(courier.MsgWired)},
	{Label: "Invalid JSON sent Status", URL: sentStatusURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Missing Keys sent Status", URL: sentStatusURL, Data: missingRequiredKeys, Status: 400, Response: "validation for 'CollerationID' failed on the 'required'"},

	{Label: "Delivered Status Valid", URL: deliveredStatusURL, Data: validDeliveredStatus, Status: 200, Response: "Status Update Accepted", MsgStatus: Sp(courier.MsgDelivered)},
	{Label: "Unknown Delivered Status Valid", URL: deliveredStatusURL, Data: unknownDeliveredStatus, Status: 400, Response: "unknown delivered status code", MsgStatus: Sp(courier.MsgSent)},
	{Label: "Invalid JSON delivered Statu", URL: deliveredStatusURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Missing Keys sent Status", URL: deliveredStatusURL, Data: missingRequiredKeys, Status: 400, Response: "validation for 'CollerationID' failed on the 'required'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "external1",
		ResponseBody:   `{"id": "external1"}`,
		ResponseStatus: 200,
		Headers:        map[string]string{"username": "user1", "authenticationtoken": "token", "Accept": "application/json", "Content-Type": "application/json"},
		RequestBody:    `{"destination":"250788383383","messageText":"Simple Message ☺\nhttps://foo.bar/image.jpg"}`,
		SendPrep:       setSendURL},
	{Label: "Error status 403",
		Text: "Error Response", URN: "tel:+250788383383",
		Status:      "E",
		RequestBody: `{"destination":"250788383383","messageText":"Error Response"}`, ResponseStatus: 403,
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `Bad Gateway`, ResponseStatus: 501,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WV", "2020", "BR",
		map[string]interface{}{
			courier.ConfigUsername:  "user1",
			courier.ConfigAuthToken: "token",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
