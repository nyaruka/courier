package clicksend

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	receiveURL = "/c/cs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CS", "2020", "US", nil),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 `from=639171234567&body=hello+world`,
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "tel:+639171234567",
	},
	{
		Label:                "Receive Missing From",
		URL:                  receiveURL,
		Data:                 `body=hello+world`,
		Headers:              map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Error",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
}

const successResponse = `{
	"http_code": 200,
	"response_code": "SUCCESS",
	"response_msg": "Here are your data.",
	"data": {
	  "total_price": 0.28,
	  "total_count": 2,
	  "queued_count": 2,
	  "messages": [
		{
		  "direction": "out",
		  "date": 1436871253,
		  "to": "+61411111111",
		  "body": "Jelly liquorice marshmallow candy carrot cake 4Eyffjs1vL.",
		  "from": "sendmobile",
		  "schedule": 1436874701,
		  "message_id": "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		  "message_parts": 1,
		  "message_price": 0.07,
		  "custom_string": "this is a test",
		  "user_id": 1,
		  "subaccount_id": 1,
		  "country": "AU",
		  "carrier": "Telstra",
		  "status": "SUCCESS"
		}
	]
}`

const failureResponse = `{
	"http_code": 200,
	"response_code": "SUCCESS",
	"response_msg": "Here are your data.",
	"data": {
	  "total_price": 0.28,
	  "total_count": 2,
	  "queued_count": 2,
	  "messages": [
		{
		  "direction": "out",
		  "date": 1436871253,
		  "to": "+61411111111",
		  "body": "Jelly liquorice marshmallow candy carrot cake 4Eyffjs1vL.",
		  "from": "sendmobile",
		  "schedule": 1436874701,
		  "message_id": "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		  "message_parts": 1,
		  "message_price": 0.07,
		  "custom_string": "this is a test",
		  "user_id": 1,
		  "subaccount_id": 1,
		  "country": "AU",
		  "carrier": "Telstra",
		  "status": "FAILURE"
		}
	]
}`

var sendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    successResponse,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"Simple Message","source":"courier"}]}`,
		ExpectedHeaders:     map[string]string{"Authorization": "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="},
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "tel:+250788383383",
		MockResponseBody:    successResponse,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"☺","source":"courier"}]}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Attachment",
		MsgText:             "My pic!",
		MsgURN:              "tel:+250788383383",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    successResponse,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"My pic!\nhttps://foo.bar/image.jpg","source":"courier"}]}`,
		ExpectedExternalID:  "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Sending",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   `[{"Response": "101"}]`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Failure Response",
		MsgText:            "Error Sending",
		MsgURN:             "tel:+250788383383",
		MockResponseBody:   failureResponse,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueUnexpected("status", "SUCCESS")},
		SendPrep:           setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US",
		map[string]any{
			"username": "Aladdin",
			"password": "open sesame",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), sendTestCases, []string{httpx.BasicAuth("Aladdin", "open sesame")}, nil)
}
