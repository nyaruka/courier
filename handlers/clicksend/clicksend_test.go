package clicksend

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL = "/c/cs/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "CS", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: `from=639171234567&body=hello+world`, Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Status: 200, Response: "Accepted", Text: Sp("hello world"), URN: Sp("tel:+639171234567")},
	{Label: "Receive Missing From", URL: receiveURL, Data: `body=hello+world`, Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Status: 400, Response: "Error"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "?%s"
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

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "tel:+250788383383",
		Status:       "W",
		Headers:      map[string]string{"Authorization": "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="},
		ResponseBody: successResponse, ResponseStatus: 200, ExternalID: "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		RequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"Simple Message","source":"courier"}]}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "tel:+250788383383",
		Status:       "W",
		ResponseBody: successResponse, ResponseStatus: 200, ExternalID: "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		RequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"☺","source":"courier"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+250788383383", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: successResponse, ResponseStatus: 200, ExternalID: "BF7AD270-0DE2-418B-B606-71D527D9C1AE",
		RequestBody: `{"messages":[{"to":"+250788383383","from":"2020","body":"My pic!\nhttps://foo.bar/image.jpg","source":"courier"}]}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: `[{"Response": "101"}]`, ResponseStatus: 403,
		SendPrep: setSendURL},
	{Label: "Failure Response",
		Text: "Error Sending", URN: "tel:+250788383383",
		Status:       "E",
		ResponseBody: failureResponse, ResponseStatus: 200,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "GL", "2020", "US",
		map[string]interface{}{
			"username": "Aladdin",
			"password": "open sesame",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), sendTestCases, nil)
}
