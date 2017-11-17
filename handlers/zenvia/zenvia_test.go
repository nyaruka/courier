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

	{Label: "Not JSON body", URL: receiveURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: receiveURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: receiveURL, Data: missingFieldsReceive, Status: 400, Response: "validation for 'ID' failed on the 'required'"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `"status":"D"`},
	{Label: "Valid Status with more fields", URL: statusURL, Data: validWithMoreFieldsStatus, Status: 200, Response: `"status":"D"`},
	{Label: "Not JSON body", URL: statusURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: statusURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: statusURL, Data: missingFieldsStatus, Status: 400, Response: "validation for 'StatusCode' failed on the 'required'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}

// setSendURL takes care of setting the sendURL to call
func setSendURL(server *httptest.Server, channel courier.Channel, msg courier.Msg) {
	sendURL = server.URL
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
		RequestBody: `{"sendSmsRequest":{"from":"Sender","to":"250788383383","schedule":"","msg":"Simple Message ☺","callbackOption":"1","id":"10","aggregateId":""}}`,
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
		RequestBody: `{"sendSmsRequest":{"from":"Sender","to":"250788383383","schedule":"","msg":"My pic!\nhttps://foo.bar/image.jpg","callbackOption":"1","id":"10","aggregateId":""}}`,
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
		RequestBody: `{"sendSmsRequest":{"from":"Sender","to":"250788383383","schedule":"","msg":"No External ID","callbackOption":"1","id":"10","aggregateId":""}}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		Error:          "received non 200 status: 401",
		RequestBody:    `{"sendSmsRequest":{"from":"Sender","to":"250788383383","schedule":"","msg":"Error Message","callbackOption":"1","id":"10","aggregateId":""}}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZV", "2020", "BR", map[string]interface{}{"username": "zv-username", "password": "zv-password"})
	RunChannelSendTestCases(t, defaultChannel, NewHandler(), defaultSendTestCases)
}
