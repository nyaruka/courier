package chikka

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CK", "2020", "US", nil),
}

const (
	receiveURL = "/c/ck/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testCases = []ChannelHandleTestCase{
	{
		Label:              "Receive Valid",
		URL:                receiveURL,
		Data:               "message_type=incoming&mobile_number=639178020779&request_id=4004&message=Hello+World&timestamp=1457670059.69",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "Message Accepted",
		ExpectedMsgText:    Sp("Hello World"),
		ExpectedURN:        "tel:+639178020779",
		ExpectedExternalID: "4004",
		ExpectedDate:       time.Date(2016, 03, 11, 04, 20, 59, 690000128, time.UTC),
	},
	{
		Label:              "Invalid URN",
		URL:                receiveURL,
		Data:               "message_type=incoming&mobile_number=MTN&request_id=4004&message=Hello+World&timestamp=1457670059.69",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "phone number supplied is not a number",
	},
	{
		Label:              "Receive Mising Params",
		URL:                receiveURL,
		Data:               "message_type=incoming&message=Hello+World&timestamp=1457670059.69",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   "Field validation for 'RequestID' failed",
	},
	{
		Label:              "Ignore Invalid message_type",
		URL:                receiveURL,
		Data:               "message_type=invalid",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   "unknown message_type request",
	},
	{
		Label:              "Status Sent Valid",
		URL:                receiveURL,
		Data:               "message_type=outgoing&message_id=10&status=SENT",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   `"status":"S"`,
	},
	{
		Label:              "Status Failed Valid",
		URL:                receiveURL,
		Data:               "message_type=outgoing&message_id=10&status=FAILED",
		ExpectedRespStatus: 200,
		ExpectedRespBody:   `"status":"F"`,
	},
	{
		Label:              "Status Invalid",
		URL:                receiveURL,
		Data:               "message_type=outgoing&message_id=10&status=UNKNOWN",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   `must be either 'SENT' or 'FAILED'`,
	},
	{
		Label:              "Status Missing Params",
		URL:                receiveURL,
		Data:               "message_type=outgoing",
		ExpectedRespStatus: 400,
		ExpectedRespBody:   `Field validation for 'Status' failed `,
	},
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
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "tel:+63911231234",
		MockResponseBody:   "Success",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"message":       "Simple Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:                   "Plain Reply",
		MsgText:                 "Simple Message",
		MsgURN:                  "tel:+63911231234",
		MsgResponseToExternalID: "external-id",
		MockResponseBody:        "Success",
		MockResponseStatus:      200,
		ExpectedPostParams: map[string]string{
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
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:                   "Failed Reply use Send",
		MsgText:                 "Simple Message",
		MsgURN:                  "tel:+63911231234",
		MsgResponseToExternalID: "external-id",
		MockResponseBody:        `{"status":400,"message":"BAD REQUEST","description":"Invalid\\/Used Request ID"}`,
		MockResponseStatus:      400,
		ExpectedPostParams: map[string]string{
			"message":       "Simple Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		SendPrep: setSendURL,
	},
	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "tel:+63911231234",
		MockResponseBody:   "Success",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"message":       "☺",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Long Send",
		MsgText:            "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:             "tel:+63911231234",
		ExpectedMsgStatus:  "W",
		MockResponseBody:   "Success",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
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
	{
		Label:              "Send Attachment",
		MsgText:            "My pic!",
		MsgURN:             "tel:+63911231234",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   "Success",
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"message":       "My pic!\nhttps://foo.bar/image.jpg",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Message",
		MsgURN:             "tel:+63911231234",
		MockResponseBody:   `ERROR`,
		MockResponseStatus: 401,
		ExpectedPostParams: map[string]string{
			"message":       "Error Message",
			"message_type":  "SEND",
			"mobile_number": "63911231234",
			"shortcode":     "2020",
			"request_cost":  "FREE",
			"client_id":     "Username",
			"secret_key":    "Password",
			"message_id":    "10",
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CK", "2020", "US",
		map[string]interface{}{
			courier.ConfigPassword: "Password",
			courier.ConfigUsername: "Username",
		})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
