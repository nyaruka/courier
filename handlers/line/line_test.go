package line

import (
	"net/http"
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
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US",
		map[string]interface{}{
			"secret": "Secret",
		}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Hello, world"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessageLast, Status: 200, Response: "Accepted",
		Text: Sp("Last event"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Missing message", URL: receiveURL, Data: missingMessage, Status: 200, Response: "ignoring request, no message",
		PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: receiveURL, Data: invalidURN, Status: 400, Response: "invalid line id",
		PrepRequest: addValidSignature},
	{Label: "No event request", URL: receiveURL, Data: noEvent, Status: 200, Response: "ignoring request, no message",
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Message Invalid signature", URL: receiveURL, Data: receiveValidMessage, Status: 400, Response: "invalid request signature",
		PrepRequest: addInvalidSignature},
}

func addValidSignature(r *http.Request) {
	sig, _ := calculateSignature("Secret", r)
	r.Header.Set(signatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	replySendURL = s.URL + "/v2/bot/message/reply"
	pushSendURL  = s.URL + "/v2/bot/message/push"
}

const tooLongMsg = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Maecenas convallis augue vel placerat congue.
Etiam nec tempus enim. Cras placerat at est vel suscipit. Duis quis faucibus metus, non elementum tortor.
Pellentesque posuere ullamcorper metus auctor venenatis. Proin eget hendrerit dui. Sed eget massa nec mauris consequat pretium.
Praesent mattis arcu tortor, ac aliquet turpis tincidunt eu.

Fusce ut lacinia augue. Vestibulum felis nisi, porta ut est condimentum, condimentum volutpat libero.
Suspendisse a elit venenatis, condimentum sem at, ultricies mauris. Morbi interdum sem id tempor tristique.
Ut tincidunt massa eu purus lacinia sodales a volutpat neque. Cras dolor quam, eleifend a rhoncus quis, sodales nec purus.
Vivamus justo dolor, gravida at quam eu, hendrerit rutrum justo. Sed hendrerit nisi vitae nisl ornare tristique.
Proin vulputate id justo non aliquet.`

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
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Image Attachment",
		Text: "My pic!", URN: "line:uabcdefghij", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"My pic!"},{"type":"image","originalContentUrl":"https://foo.bar/image.jpg","previewImageUrl":"https://foo.bar/image.jpg"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Other Attachment",
		Text: "My video!", URN: "line:uabcdefghij", Attachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"My video!"},{"type":"text","text":"https://foo.bar/video.mp4"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Message Batches",
		Text:         tooLongMsg,
		URN:          "line:uabcdefghij",
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Sed hendrerit nisi vitae nisl ornare tristique.\nProin vulputate id justo non aliquet."}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Reply Message",
		Text: "Simple Message", URN: "line:uabcdefghij", ResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		Status:       "W",
		ResponseBody: `{}`, ResponseStatus: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		RequestBody: `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
		SendPrep:    setSendURL},
	{Label: "Send Push Message If Invalid Reply",
		Text: "Simple Message", URN: "line:uabcdefghij", ResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		Status: "W",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/v2/bot/message/reply",
				BodyContains: `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: {
				Status: 400,
				Body: `{"message":"Invalid reply token"}`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/v2/bot/message/push",
				BodyContains: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: {
				Status: 200,
				Body: `{}`,
			},
		},
		SendPrep: setSendURL},
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
