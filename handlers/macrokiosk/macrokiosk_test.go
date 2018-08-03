package macrokiosk

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MK", "2020", "MY", nil),
}

var (
	receiveURL = "/c/mk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/mk/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	validReceive         = "shortcode=2020&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	invalidURN           = "shortcode=2020&from=MTN&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	validLongcodeReceive = "longcode=2020&msisdn=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	missingParamsReceive = "from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	invalidParamsReceive = "longcode=2020&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"
	invalidAddress       = "shortcode=1515&from=%2B60124361111&text=Hello&msgid=abc1234&time=2016-03-3019:33:06"

	validStatus      = "msgid=12345&status=ACCEPTED"
	processingStatus = "msgid=12345&status=PROCESSING"
	unknownStatus    = "msgid=12345&status=UNKNOWN"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Receive Valid via GET", URL: receiveURL + "?" + validReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Receive Valid", URL: receiveURL, Data: validLongcodeReceive, Status: 200, Response: "-1",
		Text: Sp("Hello"), URN: Sp("tel:+60124361111"), Date: Tp(time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC)),
		ExternalID: Sp("abc1234")},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Missing Params", URL: receiveURL, Data: missingParamsReceive, Status: 400, Response: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Params", URL: receiveURL, Data: invalidParamsReceive, Status: 400, Response: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, Status: 400, Response: "invalid to number [1515], expecting [2020]"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `"status":"S"`},
	{Label: "Wired Status", URL: statusURL, Data: processingStatus, Status: 200, Response: `"status":"W"`},
	{Label: "Unknown Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: `ignoring unknown status 'UNKNOWN'`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "MsgID":"abc123" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"Simple Message ☺","from":"macro","servid":"service-id","type":"5"}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "MsgID":"abc123" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"I need to keep adding more things to make it work","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "tel:+250788383383",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "abc123",
		ResponseBody:   `{ "MsgID":"abc123" }`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"My pic!\nhttps://foo.bar/image.jpg","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:    setSendURL},
	{Label: "No External Id",
		Text:           "No External ID",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "missing":"OzYDlvf3SQVc" }`,
		ResponseStatus: 200,
		Error:          "unable to parse response body from Macrokiosk",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		RequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"No External ID","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		RequestBody:    `{"user":"Username","pass":"Password","to":"250788383383","text":"Error Message","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MK", "2020", "US",
		map[string]interface{}{
			"password":                "Password",
			"username":                "Username",
			configMacrokioskSenderID:  "macro",
			configMacrokioskServiceID: "service-id",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
