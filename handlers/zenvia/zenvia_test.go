package zenvia

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZV", "2020", "BR", map[string]interface{}{"username": "zv-username", "password": "zv-password"}),
}

var (
	receiveURL = "/c/zv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/zv/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	notJSON = "empty"
)

var wrongJSONSchema = `{}`

var validWithMoreFieldsStatus = `{
	"callbackMtRequest": {
        "status": "03",
        "statusMessage": "Delivered",
        "statusDetail": "120",
        "statusDetailMessage": "Message received by mobile",
        "id": "hs765939216",
        "received": "2014-08-26T12:55:48.593-03:00",
        "mobileOperatorName": "Claro"
    }
}`

var validStatus = `{
    "callbackMtRequest": {
        "status": "03",
        "id": "hs765939216"
    }
}`

var unknownStatus = `{
    "callbackMtRequest": {
        "status": "038",
        "id": "hs765939216"
    }
}`

var missingFieldsStatus = `{
	"callbackMtRequest": {
        "status": "",
        "id": "hs765939216"
    }
}`

var validReceive = `{
    "callbackMoRequest": {
        "id": "20690090",
        "mobile": "254791541111",
        "shortCode": "40001",
        "account": "zenvia.envio",
        "body": "Msg",
        "received": "2017-05-03T03:04:45.123-03:00",
        "correlatedMessageSmsId": "hs765939061"
    }
}`

var invalidURN = `{
    "callbackMoRequest": {
        "id": "20690090",
        "mobile": "MTN",
        "shortCode": "40001",
        "account": "zenvia.envio",
        "body": "Msg",
        "received": "2017-05-03T03:04:45.123-03:00",
        "correlatedMessageSmsId": "hs765939061"
    }
}`

var invalidDateReceive = `{
    "callbackMoRequest": {
        "id": "20690090",
        "mobile": "254791541111",
        "shortCode": "40001",
        "account": "zenvia.envio",
        "body": "Msg",
        "received": "yesterday?",
        "correlatedMessageSmsId": "hs765939061"
    }
}`

var missingFieldsReceive = `{
	"callbackMoRequest": {
        "id": "",
        "mobile": "254791541111",
        "shortCode": "40001",
        "account": "zenvia.envio",
        "body": "Msg",
        "received": "2017-05-03T03:04:45.123-03:00",
        "correlatedMessageSmsId": "hs765939061"
    }
}`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("tel:+254791541111"), Date: Tp(time.Date(2017, 5, 3, 06, 04, 45, 123000000, time.UTC))},

	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "phone number supplied is not a number"},
	{Label: "Not JSON body", URL: receiveURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: receiveURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: receiveURL, Data: missingFieldsReceive, Status: 400, Response: "validation for 'ID' failed on the 'required'"},
	{Label: "Bad Date", URL: receiveURL, Data: invalidDateReceive, Status: 400, Response: "invalid date format"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `Accepted`, MsgStatus: Sp("D")},
	{Label: "Valid Status with more fields", URL: statusURL, Data: validWithMoreFieldsStatus, Status: 200, Response: `Accepted`, MsgStatus: Sp("D")},
	{Label: "Unkown Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: "Accepted", MsgStatus: Sp("E")},
	{Label: "Not JSON body", URL: statusURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: statusURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: statusURL, Data: missingFieldsStatus, Status: 400, Response: "validation for 'StatusCode' failed on the 'required'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text:           "Simple Message ☺",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{"sendSmsResponse":{"statusCode":"00","statusDescription":"Ok","detailCode":"000","detailDescription":"Message Sent"}}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		RequestBody: `{"sendSmsRequest":{"to":"250788383383","schedule":"","msg":"Simple Message ☺","callbackOption":"FINAL","id":"10","aggregateId":""}}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{"sendSmsResponse":{"statusCode":"00","statusDescription":"Ok","detailCode":"000","detailDescription":"Message Sent"}}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		RequestBody: `{"sendSmsRequest":{"to":"250788383383","schedule":"","msg":"I need to keep adding more things to make it work","callbackOption":"FINAL","id":"10","aggregateId":""}}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "tel:+250788383383",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "",
		ResponseBody:   `{"sendSmsResponse":{"statusCode":"00","statusDescription":"Ok","detailCode":"000","detailDescription":"Message Sent"}}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		RequestBody: `{"sendSmsRequest":{"to":"250788383383","schedule":"","msg":"My pic!\nhttps://foo.bar/image.jpg","callbackOption":"FINAL","id":"10","aggregateId":""}}`,
		SendPrep:    setSendURL},
	{Label: "No External Id",
		Text:           "No External ID",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{"sendSmsResponse" :{"statusCode" :"05","statusDescription" :"Blocked","detailCode":"140","detailDescription":"Mobile number not covered"}}`,
		ResponseStatus: 200,
		Error:          "received non-success response from Zenvia '05'",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Basic enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
		},
		RequestBody: `{"sendSmsRequest":{"to":"250788383383","schedule":"","msg":"No External ID","callbackOption":"FINAL","id":"10","aggregateId":""}}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		RequestBody:    `{"sendSmsRequest":{"to":"250788383383","schedule":"","msg":"Error Message","callbackOption":"FINAL","id":"10","aggregateId":""}}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZV", "2020", "BR", map[string]interface{}{"username": "zv-username", "password": "zv-password"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
