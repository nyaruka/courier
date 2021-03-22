package zenviawhatsapp

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVW", "2020", "BR", map[string]interface{}{"api_key": "zv-api-token"}),
}

var (
	receiveURL = "/c/zvw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"
	statusURL  = "/c/zvw/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"

	notJSON = "empty"
)

var wrongJSONSchema = `{}`

var validStatus = `{
	"id": "string",
	"type": "MESSAGE_STATUS",
	"channel": "string",
	"messageId": "hs765939216",
	"messageStatus": {
	  "timestamp": "2021-03-12T12:15:31Z",
	  "code": "SENT"
	}
}`

var unknownStatus = `{
	"id": "string",
	"type": "MESSAGE_STATUS",
	"channel": "string",
	"messageId": "hs765939216",
	"messageStatus": {
	  "timestamp": "2021-03-12T12:15:31Z",
	  "code": "FOO"
	}
}`

var invalidTypeStatus = `{
	"id": "string",
	"type": "MESSAGE_REPORT",
	"channel": "string",
	"messageId": "hs765939216",
	"messageStatus": {
	  "timestamp": "2021-03-12T12:15:31Z",
	  "code": "SENT"
	}
}`

var validReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var fileReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "file",
		  "fileUrl": "https://foo.bar/v1/media/41"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var locationReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "location",
		  "longitude": 1.00,
		  "latitude": 0.00
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var invalidURN = `{
  "id": "string",
  "timestamp": "2017-05-03T03:04:45Z",
  "type": "MESSAGE",
  "message": {
    "id": "string",
    "from": "MTN",
    "to": "2020",
    "direction": "IN",
    "contents": [
       {
         "type": "text",
         "text": "Msg",
         "payload": "string"
       }
    ],
    "visitor": {
  	"name": "Bob"
    }
  }
}`

var invalidDateReceive = `{
	"id": "string",
	"timestamp": "2014-08-26T12:55:48.593-03:00",
	"type": "MESSAGE",
	"message": {
	  "id": "string",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
  }`

var missingFieldsReceive = `{
	"id": "string",
	"timestamp": "2017-05-03T03:04:45Z",
	"type": "MESSAGE",
	"message": {
	  "id": "",
	  "from": "254791541111",
	  "to": "2020",
	  "direction": "IN",
	  "contents": [
		{
		  "type": "text",
		  "text": "Msg",
		  "payload": "string"
		}
	  ],
	  "visitor": {
		"name": "Bob"
	  }
	}
}`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid", URL: receiveURL, Data: validReceive, Status: 200, Response: "Message Accepted",
		Text: Sp("Msg"), URN: Sp("whatsapp:254791541111"), Date: Tp(time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC))},

	{Label: "Receive file Valid", URL: receiveURL, Data: fileReceive, Status: 200, Response: "Message Accepted",
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:254791541111"), Date: Tp(time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC))},

	{Label: "Receive location Valid", URL: receiveURL, Data: locationReceive, Status: 200, Response: "Message Accepted",
		Text: Sp(""), Attachment: Sp("geo:0.000000,1.000000"), URN: Sp("whatsapp:254791541111"), Date: Tp(time.Date(2017, 5, 3, 03, 04, 45, 0, time.UTC))},

	{Label: "Not JSON body", URL: receiveURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: receiveURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
	{Label: "Missing field", URL: receiveURL, Data: missingFieldsReceive, Status: 400, Response: "validation for 'ID' failed on the 'required'"},
	{Label: "Bad Date", URL: receiveURL, Data: invalidDateReceive, Status: 400, Response: "invalid date format"},

	{Label: "Valid Status", URL: statusURL, Data: validStatus, Status: 200, Response: `Accepted`, MsgStatus: Sp("S")},
	{Label: "Unkown Status", URL: statusURL, Data: unknownStatus, Status: 200, Response: "Accepted", MsgStatus: Sp("E")},
	{Label: "Not JSON body", URL: statusURL, Data: notJSON, Status: 400, Response: "unable to parse request JSON"},
	{Label: "Wrong JSON schema", URL: statusURL, Data: wrongJSONSchema, Status: 400, Response: "request JSON doesn't match required schema"},
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
		ExternalID:     "55555",
		ResponseBody:   `{"id": "55555"}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		RequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Simple Message ☺"}]}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:           "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:            "tel:+250788383383",
		Status:         "W",
		ExternalID:     "55555",
		ResponseBody:   `{"id": "55555"}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		RequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text:           "My pic!",
		URN:            "tel:+250788383383",
		Attachments:    []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:         "W",
		ExternalID:     "55555",
		ResponseBody:   `{"id": "55555"}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		RequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"file","fileUrl":"https://foo.bar/image.jpg","fileMimeType":"image/jpeg"},{"type":"text","text":"My pic!"}]}`,
		SendPrep:    setSendURL},
	{Label: "No External ID",
		Text:           "No External ID",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{"code": "400","message": "Validation error","details": [{"code": "400","path": "Error","message": "Error description"}]}`,
		ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
			"X-API-TOKEN":  "zv-api-token",
		},
		RequestBody: `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"No External ID"}]}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text:           "Error Message",
		URN:            "tel:+250788383383",
		Status:         "E",
		ResponseBody:   `{ "error": "failed" }`,
		ResponseStatus: 401,
		RequestBody:    `{"from":"2020","to":"250788383383","contents":[{"type":"text","text":"Error Message"}]}`,
		SendPrep:       setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ZVW", "2020", "BR", map[string]interface{}{"api_key": "zv-api-token"})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
