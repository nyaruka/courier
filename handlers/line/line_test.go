package line

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL = "/c/ln/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive"
)

var receiveValidMessage = `
{
	"events": [{
		"replyToken": "abcdefghij",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabcdefghij"
		},
		"message": {
			"id": "100001",
			"type": "text",
			"text": "Hello, world"
		}
	}, {
		"replyToken": "abcdefghijklm",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabcdefghij"
		},
		"message": {
			"id": "100002",
			"type": "sticker",
			"packageId": "1",
			"stickerId": "1"
		}
	}]
}`

var invalidURN = `
{
	"events": [{
		"replyToken": "abcdefghij",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabc!!$$defghij"
		},
		"message": {
			"id": "100001",
			"type": "text",
			"text": "Hello, world"
		}
	}, {
		"replyToken": "abcdefghijklm",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabc!!$$defghij"
		},
		"message": {
			"id": "100002",
			"type": "sticker",
			"packageId": "1",
			"stickerId": "1"
		}
	}]
}`

var receiveValidMessageLast = `
{
	"events": [{
		"replyToken": "abcdefghijklm",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabcdefghij"
		},
		"message": {
			"id": "100002",
			"type": "sticker",
			"packageId": "1",
			"stickerId": "1"
		}
	}, {
		"replyToken": "abcdefghij",
		"type": "message",
		"timestamp": 1459991487970,
		"source": {
			"type": "user",
			"userId": "uabcdefghij"
		},
		"message": {
			"id": "100001",
			"type": "text",
			"text": "Last event"
		}
	}]
}`

var missingMessage = `{
	"events": [{
		"replyToken": "abcdefghij",
		"type": "message",
		"timestamp": 1451617200000,
		"source": {
			"type": "user",
			"userId": "uabcdefghij"
		}
	}]
}`

var noEvent = `{
	"events": []
}`

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US", nil),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Hello, world"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessageLast, Status: 200, Response: "Accepted",
		Text: Sp("Last event"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC))},
	{Label: "Missing message", URL: receiveURL, Data: missingMessage, Status: 200, Response: "ignoring request, no message"},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "invalid line id"},
	{Label: "No event request", URL: receiveURL, Data: noEvent, Status: 200, Response: "ignoring request, no message"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "line:uabcdefghij",
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "Simple Message ☺", URN: "line:uabcdefghij",
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message ☺"}]}`,
		SendPrep:    setSendURL},
	{Label: "Long Send",
		Text:         "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		URN:          "line:uabcdefghij",
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Attachment",
		Text: "My pic!", URN: "line:uabcdefghij", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"My pic!\nhttps://foo.bar/image.jpg"}]}`,
		SendPrep:    setSendURL},
	{Label: "Error Sending",
		Text: "Error Sending", URN: "line:uabcdefghij",
		Status:       "E",
		ResponseBody: `{"message": "Error"}`, ResponseStatus: 403,
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Error Sending"}]}`,
		SendPrep:    setSendURL},
}

func TestSending(t *testing.T) {
	maxMsgLength = 160
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US",
		map[string]interface{}{
			"auth_token": "AccessToken",
		},
	)

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
