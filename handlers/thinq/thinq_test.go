package thinq

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US", nil),
}

var (
	receiveURL = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	receiveValid = "message=hello+world&from=2065551234&type=sms&to=2065551212"
	receiveMedia = "message=http://foo.bar/foo.png&hello+world&from=2065551234&type=mms&to=2065551212"

	statusURL     = "/c/tq/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
	statusValid   = "guid=1234&status=DELIVRD"
	statusInvalid = "guid=1234&status=UN"
	missingGUID   = "status=DELIVRD"
)

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: receiveValid, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("tel:+12065551234")},
	{Label: "Receive No Params", URL: receiveURL, Data: " ", Status: 400, Response: `'From' failed on the 'required'`},
	{Label: "Receive Media", URL: receiveURL, Data: receiveMedia, Status: 200, Response: "Accepted",
		URN: Sp("tel:+12065551234"), Attachments: []string{"http://foo.bar/foo.png"}},

	{Label: "Status Valid", URL: statusURL, Data: statusValid, Status: 200,
		ExternalID: Sp("1234"), Response: `"status":"D"`},
	{Label: "Status Invalid", URL: statusURL, Data: statusInvalid, Status: 400,
		ExternalID: Sp("1234"), Response: `"unknown status: 'UN'"`},
	{Label: "Status Missing GUID", URL: statusURL, Data: missingGUID, Status: 400,
		ExternalID: Sp("1234"), Response: `'GUID' failed on the 'required' tag`},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL + "?account_id=%s"
	sendMMSURL = s.URL + "?account_id=%s"
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+12067791234",
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "guid": "1002" }`, ResponseStatus: 200,
		Headers:     map[string]string{"Authorization": "Basic dXNlcjE6c2VzYW1l"},
		RequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Simple Message ☺"}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "tel:+12067791234", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "guid": "1002" }`, ResponseStatus: 200,
		RequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"My pic!"}`,
		SendPrep:    setSendURL},
	{Label: "Only Attachment",
		Text: "", URN: "tel:+12067791234", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "1002",
		ResponseBody: `{ "guid": "1002" }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "No External ID",
		Text: "No External ID", URN: "tel:+12067791234",
		Status:       "E",
		ResponseBody: `{}`, ResponseStatus: 200,
		RequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"No External ID"}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Message", URN: "tel:+12067791234",
		Status:       "E",
		ResponseBody: `{ "error": "failed" }`, ResponseStatus: 401,
		RequestBody: `{"from_did":"2065551212","to_did":"2067791234","message":"Error Message"}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	var channel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TQ", "+12065551212", "US",
		map[string]interface{}{
			configAccountID:    "1234",
			configAPITokenUser: "user1",
			configAPIToken:     "sesame",
		})
	RunChannelSendTestCases(t, channel, newHandler(), sendTestCases, nil)
}
