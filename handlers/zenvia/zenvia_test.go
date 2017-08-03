package zenvia

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZV", "2020", "BR", map[string]interface{}{"username": "zv_account", "password": "zv-code"}),
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
