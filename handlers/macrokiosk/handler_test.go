package macrokiosk

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MK", "2020", "MY", nil),
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

var incomingTestCases = []IncomingTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "-1",
		ExpectedMsgText: Sp("Hello"), ExpectedURN: "tel:+60124361111", ExpectedDate: time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC),
		ExpectedExternalID: "abc1234"},
	{Label: "Receive Valid via GET", URL: receiveURL + "?" + validReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "-1",
		ExpectedMsgText: Sp("Hello"), ExpectedURN: "tel:+60124361111", ExpectedDate: time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC),
		ExpectedExternalID: "abc1234"},
	{Label: "Receive Valid", URL: receiveURL, Data: validLongcodeReceive, ExpectedRespStatus: 200, ExpectedBodyContains: "-1",
		ExpectedMsgText: Sp("Hello"), ExpectedURN: "tel:+60124361111", ExpectedDate: time.Date(2016, 3, 30, 11, 33, 06, 0, time.UTC),
		ExpectedExternalID: "abc1234"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, ExpectedRespStatus: 400, ExpectedBodyContains: "phone number supplied is not a number"},
	{Label: "Missing Params", URL: receiveURL, Data: missingParamsReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Params", URL: receiveURL, Data: invalidParamsReceive, ExpectedRespStatus: 400, ExpectedBodyContains: "missing shortcode, longcode, from or msisdn parameters"},
	{Label: "Invalid Address Params", URL: receiveURL, Data: invalidAddress, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid to number [1515], expecting [2020]"},

	{
		Label:                "Valid Status",
		URL:                  statusURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"S"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12345", Status: courier.MsgStatusSent},
		},
	},
	{
		Label:                "Wired Status",
		URL:                  statusURL,
		Data:                 processingStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"status":"W"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "12345", Status: courier.MsgStatusWired},
		},
	},
	{Label: "Unknown Status", URL: statusURL, Data: unknownStatus, ExpectedRespStatus: 200, ExpectedBodyContains: `ignoring unknown status 'UNKNOWN'`},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), incomingTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

var outgoingTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message ☺",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "MsgID":"abc123" }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"Simple Message ☺","from":"macro","servid":"service-id","type":"5"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "MsgID":"abc123" }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"I need to keep adding more things to make it work","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+250788383383",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "abc123",
		MockResponseBody:   `{ "MsgID":"abc123" }`,
		MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"My pic!\nhttps://foo.bar/image.jpg","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:              "No External Id",
		MsgText:            "No External ID",
		MsgURN:             "tel:+250788383383",
		ExpectedMsgStatus:  "E",
		MockResponseBody:   `{ "missing":"OzYDlvf3SQVc" }`,
		MockResponseStatus: 200,
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "unable to parse response body from Macrokiosk")},
		ExpectedHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		ExpectedRequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"No External ID","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Message",
		MsgURN:              "tel:+250788383383",
		ExpectedMsgStatus:   "E",
		MockResponseBody:    `{ "error": "failed" }`,
		MockResponseStatus:  401,
		ExpectedRequestBody: `{"user":"Username","pass":"Password","to":"250788383383","text":"Error Message","from":"macro","servid":"service-id","type":"0"}`,
		SendPrep:            setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MK", "2020", "US",
		map[string]any{
			"password":                "Password",
			"username":                "Username",
			configMacrokioskSenderID:  "macro",
			configMacrokioskServiceID: "service-id",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), outgoingTestCases, []string{"Password"}, nil)
}
