package plivo

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "MY", nil),
}

var (
	receiveURL = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive   = "To=2020&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	invalidURN     = "To=2020&From=MTN&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	invalidAddress = "To=1515&From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"
	missingParams  = "From=60124361111&TotalRate=0&Units=1&Text=Hello&TotalAmount=0&Type=sms&MessageUUID=abc1234"

	validStatus          = "MessageUUID=12345&status=delivered&To=%2B60124361111&From=2020"
	validSentStatus      = "ParentMessageUUID=12345&status=sent&MessageUUID=123&To=%2B60124361111&From=2020"
	invalidStatusAddress = "ParentMessageUUID=12345&status=sent&MessageUUID=123&To=%2B60124361111&From=1515"
	unknownStatus        = "MessageUUID=12345&status=UNKNOWN&To=%2B60124361111&From=2020"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), ExternalID: Sp("abc1234")},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, Status: 400, Response: "invalid to number [1515], expecting [2020]"},
	{Label: "Missing Params", URL: receiveURL, Data: missingParams, Status: 400, Response: "Field validation for 'To' failed"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `"status":"D"`},
	{Label: "Sent Status", URL: statusURL, Data: validSentStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Invalid Status Address", URL: statusURL, Data: invalidStatusAddress, Status: 400, Response: "invalid to number [1515], expecting [2020]"},
	{Label: "Unkown Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: `ignoring unknown status 'UNKNOWN'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "/%s/"
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "message_uuid":["abc123"] }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		RequestBody: `{"src":"2020","dst":"250788383383","text":"Simple Message ☺","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "message_uuid":["abc123"] }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		RequestBody: `{"src":"2020","dst":"250788383383","text":"I need to keep adding more things to make it work","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "tel:+250788383383",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "message_uuid":["abc123"] }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		RequestBody: `{"src":"2020","dst":"250788383383","text":"My pic!\nhttps://foo.bar/image.jpg","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:    setSendURL},
	{Label: "No External Id",
		Text:           "No External ID",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "missing":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Error:          "unable to parse response body from Plivo",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic QXV0aElEOkF1dGhUb2tlbg==",
		},
		RequestBody: `{"src":"2020","dst":"250788383383","text":"No External ID","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		RequestBody:    `{"src":"2020","dst":"250788383383","text":"Error Message","url":"https://localhost/c/pl/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status","method":"POST"}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PL", "2020", "US",
		map[string]interface{}{
			configPlivoAuthID:    "AuthID",
			configPlivoAuthToken: "AuthToken",
			configPlivoAPPID:     "AppID",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
