package chikka

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CK", "2020", "US", nil),
}

var (
	receiveURL           = "/c/ck/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
	validReceive         = "message_type=incoming&mobile_number=639178020779&request_id=4004&message=Hello+World&timestamp=1457670059.69"
	invalidURN           = "message_type=incoming&mobile_number=MTN&request_id=4004&message=Hello+World&timestamp=1457670059.69"
	missingParamsReceive = "message_type=incoming&message=Hello+World&timestamp=1457670059.69"

	validSentStatus     = "message_type=outgoing&message_id=10&status=SENT"
	validFailedStatus   = "message_type=outgoing&message_id=10&status=FAILED"
	invalidStatus       = "message_type=outgoing&message_id=10&status=UNKNOWN"
	missingStatusParams = "message_type=outgoing"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello World"), URN: Sp("tel:+639178020779"), ExternalID: Sp("4004"),
		Date: Tp(time.Date(2016, 03, 11, 04, 20, 59, 690000128, time.UTC))},

	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Receive Mising Params", URL: receiveURL, Data: missingParamsReceive, Status: 400, Response: "Field validation for 'RequestID' failed"},
	{Label: "Ignore Invalid message_type", URL: receiveURL, Data: "message_type=invalid", Status: 200, Response: "unknown message_type request"},
	{Label: "Status Sent Valid", URL: receiveURL, Data: validSentStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Status Failed Valid", URL: receiveURL, Data: validFailedStatus, Status: 200, Response: `"status":"F"`},
	{Label: "Status Invalid", URL: receiveURL, Data: invalidStatus, Status: 400, Response: `must be either 'SENT' or 'FAILED'`},
	{Label: "Status Missing Params", URL: receiveURL, Data: missingStatusParams, Status: 400, Response: `Field validation for 'Status' failed `},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSend takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+63911231234",
		Status:       "W",
		ResponseBody: "Success", ResponseStatus: 200,
		PostParams: map[string]string{
			"message":       "Simple Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Plain Reply",
		Text: "Simple Message", URN: "tel:+63911231234",
		Status:               "W",
		ResponseToID:         5,
		ResponseToExternalID: "external-id",
		ResponseBody:         "Success", ResponseStatus: 200,
		PostParams: map[string]string{
			"message":       "Simple Message",
			"message_type":  "REPLY",
			"request_id":    "external-id",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Failed Reply use Send",
		Text: "Simple Message", URN: "tel:+63911231234",
		ResponseToID:         5,
		ResponseToExternalID: "external-id",
		ResponseBody:         `{"status":400,"message":"BAD REQUEST","description":"Invalid\\/Used Request ID"}`,
		ResponseStatus:       400,
		PostParams: map[string]string{
			"message":       "Simple Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+63911231234",
		Status:       "W",
		ResponseBody: "Success", ResponseStatus: 200,
		PostParams: map[string]string{
			"message":       "☺",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Long Send",
		Text:         "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:          "tel:+63911231234",
		Status:       "W",
		ResponseBody: "Success", ResponseStatus: 200,
		PostParams: map[string]string{
			"message":       "I need to keep adding more things to make it work",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+63911231234", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: "Success", ResponseStatus: 200,
		PostParams: map[string]string{
			"message":       "My pic!\nhttps://foo.bar/image.jpg",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+63911231234",
		Status:       "E",
		ResponseBody: `ERROR`, ResponseStatus: 401,
		PostParams: map[string]string{
			"message":       "Error Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CK", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
