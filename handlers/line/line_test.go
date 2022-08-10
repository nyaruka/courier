package line

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
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

var receiveValidImageMessage = `
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
			"type": "image",
			"contentProvider": {
				"type": "line"
			}
		}
	}]
}`

var receiveValidVideoMessage = `
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
			"type": "video",
			"contentProvider": {
				"type": "line"
			}
		}
	}]
}`

var receiveValidVideoExternalMessage = `
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
			"type": "video",
			"contentProvider": {
				"type": "external",
				"originalContentUrl": "https://example.com/original.mp4"
			}
		}
	}]
}`

var receiveValidAudioMessage = `
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
			"type": "audio",
			"contentProvider": {
				"type": "line"
			}
		}
	}]
}`

var receiveValidFileMessage = `
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
			"type": "audio"
		}
	}]
}`

var receiveValidLocationMessage = `
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
			"type": "location",
			"title": "my location",
            "address": "Japan, 〒160-0004 Tokyo, Shinjuku City, Yotsuya, 1-chōme-6-1",
            "latitude": 35.687574,
            "longitude": 139.72922
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
			"secret":     "Secret",
			"auth_token": "the-auth-token",
		}),
}

var handleTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessage, Status: 200, Response: "Accepted",
		Text: Sp("Hello, world"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Message", URL: receiveURL, Data: receiveValidMessageLast, Status: 200, Response: "Accepted",
		Text: Sp("Last event"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Image Message", URL: receiveURL, Data: receiveValidImageMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), Attachment: Sp("https://api-data.line.me/v2/bot/message/100001/content"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Video Message", URL: receiveURL, Data: receiveValidVideoMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), Attachment: Sp("https://api-data.line.me/v2/bot/message/100001/content"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Video External Message", URL: receiveURL, Data: receiveValidVideoExternalMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), Attachment: Sp("https://example.com/original.mp4"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Audio Message", URL: receiveURL, Data: receiveValidAudioMessage, Status: 200, Response: "Accepted",
		Text: Sp(""), Attachment: Sp("https://api-data.line.me/v2/bot/message/100001/content"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Location Message", URL: receiveURL, Data: receiveValidLocationMessage, Status: 200, Response: "Accepted",
		Text: Sp("my location"), Attachment: Sp("geo:35.687574,139.729220"), URN: Sp("line:uabcdefghij"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
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
	pushSendURL = s.URL + "/v2/bot/message/push"
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
		MsgText: "Simple Message", MsgURN: "line:uabcdefghij",
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
		SendPrep:            setSendURL},
	{Label: "Unicode Send",
		MsgText: "Simple Message ☺", MsgURN: "line:uabcdefghij",
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message ☺"}]}`,
		SendPrep:            setSendURL},
	{Label: "Long Send",
		MsgText:          "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:           "line:uabcdefghij",
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Image Attachment",
		MsgText: "My pic!", MsgURN: "line:uabcdefghij", MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"My pic!"},{"type":"image","originalContentUrl":"https://foo.bar/image.jpg","previewImageUrl":"https://foo.bar/image.jpg"}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Other Attachment",
		MsgText: "My video!", MsgURN: "line:uabcdefghij", MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"My video!"},{"type":"text","text":"https://foo.bar/video.mp4"}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Message Batches",
		MsgText:          tooLongMsg,
		MsgURN:           "line:uabcdefghij",
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Sed hendrerit nisi vitae nisl ornare tristique.\nProin vulputate id justo non aliquet."}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Reply Message",
		MsgText: "Simple Message", MsgURN: "line:uabcdefghij", MsgResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
		SendPrep:            setSendURL},
	{Label: "Quick Reply",
		MsgText: "Are you happy?", MsgURN: "line:uabcdefghij",
		MsgQuickReplies:  []string{"Yes", "No"},
		ExpectedStatus:   "W",
		MockResponseBody: `{}`, MockResponseStatus: 200,
		ExpectedHeaders: map[string]string{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer AccessToken",
		},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Are you happy?","quickReply":{"items":[{"type":"action","action":{"type":"message","label":"Yes","text":"Yes"}},{"type":"action","action":{"type":"message","label":"No","text":"No"}}]}}]}`,
		SendPrep:            setSendURL},
	{Label: "Send Push Message If Invalid Reply",
		MsgText: "Simple Message", MsgURN: "line:uabcdefghij", MsgResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		ExpectedStatus: "W",
		MockResponses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/v2/bot/message/reply",
				BodyContains: `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: {
				Status: 400,
				Body:   `{"message":"Invalid reply token"}`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/v2/bot/message/push",
				BodyContains: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: {
				Status: 200,
				Body:   `{}`,
			},
		},
		SendPrep: setSendURL},
	{Label: "Error Sending",
		MsgText: "Error Sending", MsgURN: "line:uabcdefghij",
		ExpectedStatus:   "E",
		MockResponseBody: `{"message": "Error"}`, MockResponseStatus: 403,
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Error Sending"}]}`,
		SendPrep:            setSendURL},
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

func TestBuildMediaRequest(t *testing.T) {
	mb := courier.NewMockBackend()

	lnHandler := &handler{NewBaseHandler(courier.ChannelType("LN"), "Line")}
	req, _ := lnHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}
