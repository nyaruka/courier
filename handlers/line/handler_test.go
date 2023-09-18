package line

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US",
		map[string]any{
			"secret":     "Secret",
			"auth_token": "the-auth-token",
		}),
}

var handleTestCases = []IncomingTestCase{
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 receiveValidMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Hello, world"),
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Message",
		URL:                  receiveURL,
		Data:                 receiveValidMessageLast,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("Last event"),
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Image Message",
		URL:                  receiveURL,
		Data:                 receiveValidImageMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://api-data.line.me/v2/bot/message/100001/content"},
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Video Message",
		URL:                  receiveURL,
		Data:                 receiveValidVideoMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://api-data.line.me/v2/bot/message/100001/content"},
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Video External Message",
		URL:                  receiveURL,
		Data:                 receiveValidVideoExternalMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://example.com/original.mp4"},
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Audio Message",
		URL:                  receiveURL,
		Data:                 receiveValidAudioMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://api-data.line.me/v2/bot/message/100001/content"},
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Location Message",
		URL:                  receiveURL,
		Data:                 receiveValidLocationMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedMsgText:      Sp("my location"),
		ExpectedAttachments:  []string{"geo:35.687574,139.729220"},
		ExpectedURN:          "line:uabcdefghij",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Missing message",
		URL:                  receiveURL,
		Data:                 missingMessage,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring request, no message",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Invalid URN",
		URL:                  receiveURL,
		Data:                 invalidURN,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid line id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No event request",
		URL:                  receiveURL,
		Data:                 noEvent,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring request, no message",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Valid Message Invalid signature",
		URL:                  receiveURL,
		Data:                 receiveValidMessage,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
}

func addValidSignature(r *http.Request) {
	sig, _ := calculateSignature("Secret", r)
	r.Header.Set(signatureHeader, string(sig))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), handleTestCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), handleTestCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
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

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "Simple Message ☺",
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message ☺"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Send",
		MsgText:             "This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say, I need to keep adding more things to make it work",
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"This is a longer message than 160 characters and will cause us to split it into two separate parts, isn't that right but it is even longer than before I say,"},{"type":"text","text":"I need to keep adding more things to make it work"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Audio Attachment",
		MsgText:             "My Audio!",
		MsgURN:              "line:uabcdefghij",
		MsgAttachments:      []string{"audio/mp3:http://mock.com/3456/test.mp3"},
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"audio","originalContentUrl":"http://mock.com/2345/test.m4a","duration":200},{"type":"text","text":"My Audio!"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Video Attachment",
		MsgText:             "My Video!",
		MsgURN:              "line:uabcdefghij",
		MsgAttachments:      []string{"video/mp4:http://mock.com/5678/test.mp4"},
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"video","originalContentUrl":"http://mock.com/5678/test.mp4","previewImageUrl":"http://mock.com/4567/test.jpg"},{"type":"text","text":"My Video!"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},

	{
		Label:               "Send Image Attachment",
		MsgText:             "My pic!",
		MsgURN:              "line:uabcdefghij",
		MsgAttachments:      []string{"image/jpeg:http://mock.com/1234/test.jpg"},
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"image","originalContentUrl":"http://mock.com/1234/test.jpg","previewImageUrl":"http://mock.com/1234/test.jpg"},{"type":"text","text":"My pic!"}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Other Attachment",
		MsgText:             "My doc!",
		MsgURN:              "line:uabcdefghij",
		MsgAttachments:      []string{"application/pdf:http://mock.com/7890/test.pdf"},
		ExpectedMsgStatus:   "W",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"http://mock.com/7890/test.pdf"},{"type":"text","text":"My doc!"}]}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Message Batches",
		MsgText:             tooLongMsg,
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Sed hendrerit nisi vitae nisl ornare tristique.\nProin vulputate id justo non aliquet."}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Send Reply Message",
		MsgText:                 "Simple Message",
		MsgURN:                  "line:uabcdefghij",
		MsgResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		MockResponseBody:        `{}`,
		MockResponseStatus:      200,
		ExpectedHeaders:         map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody:     `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
		ExpectedMsgStatus:       "W",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "line:uabcdefghij",
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Are you happy?","quickReply":{"items":[{"type":"action","action":{"type":"message","label":"Yes","text":"Yes"}},{"type":"action","action":{"type":"message","label":"No","text":"No"}}]}}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply combined and attachment",
		MsgText:             "Are you happy?",
		MsgURN:              "line:uabcdefghij",
		MsgAttachments:      []string{"image/jpeg:http://mock.com/1234/test.jpg"},
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{}`,
		MockResponseStatus:  200,
		ExpectedHeaders:     map[string]string{"Content-Type": "application/json", "Accept": "application/json", "Authorization": "Bearer AccessToken"},
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"image","originalContentUrl":"http://mock.com/1234/test.jpg","previewImageUrl":"http://mock.com/1234/test.jpg"},{"type":"text","text":"Are you happy?","quickReply":{"items":[{"type":"action","action":{"type":"message","label":"Yes","text":"Yes"}},{"type":"action","action":{"type":"message","label":"No","text":"No"}}]}}]}`,
		ExpectedMsgStatus:   "W",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Send Push Message If Invalid Reply",
		MsgText:                 "Simple Message",
		MsgURN:                  "line:uabcdefghij",
		MsgResponseToExternalID: "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/v2/bot/message/reply",
				BodyContains: `{"replyToken":"nHuyWiB7yP5Zw52FIkcQobQuGDXCTA","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: httpx.NewMockResponse(400, nil, []byte(`{"message":"Invalid reply token"}`)),
			{
				Method:       "POST",
				Path:         "/v2/bot/message/push",
				BodyContains: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Simple Message"}]}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{}`)),
		},
		ExpectedMsgStatus: "W",
		SendPrep:          setSendURL,
	},
	{
		Label:               "Invalid JSON response sending",
		MsgText:             "Error Sending",
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    ``,
		MockResponseStatus:  403,
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Error Sending"}]}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Sending",
		MsgText:             "Error Sending",
		MsgURN:              "line:uabcdefghij",
		MockResponseBody:    `{"message": "Failed to send messages"}`,
		MockResponseStatus:  403,
		ExpectedRequestBody: `{"to":"uabcdefghij","messages":[{"type":"text","text":"Error Sending"}]}`,
		ExpectedMsgStatus:   "E",
		ExpectedErrors:      []*courier.ChannelError{courier.ErrorExternal("403", "Failed to send messages")},
		SendPrep:            setSendURL,
	},
}

// setupMedia takes care of having the media files needed to our test server host
func setupMedia(mb *test.MockBackend) {
	imageJPG := test.NewMockMedia("test.jpg", "image/jpeg", "http://mock.com/1234/test.jpg", 1024*1024, 640, 480, 0, nil)

	audioM4A := test.NewMockMedia("test.m4a", "audio/mp4", "http://mock.com/2345/test.m4a", 1024*1024, 0, 0, 200, nil)
	audioMP3 := test.NewMockMedia("test.mp3", "audio/mp3", "http://mock.com/3456/test.mp3", 1024*1024, 0, 0, 200, []courier.Media{audioM4A})

	thumbJPG := test.NewMockMedia("test.jpg", "image/jpeg", "http://mock.com/4567/test.jpg", 1024*1024, 640, 480, 0, nil)
	videoMP4 := test.NewMockMedia("test.mp4", "video/mp4", "http://mock.com/5678/test.mp4", 1024*1024, 0, 0, 1000, []courier.Media{thumbJPG})

	videoMOV := test.NewMockMedia("test.mov", "video/quicktime", "http://mock.com/6789/test.mov", 100*1024*1024, 0, 0, 2000, nil)

	filePDF := test.NewMockMedia("test.pdf", "application/pdf", "http://mock.com/7890/test.pdf", 100*1024*1024, 0, 0, 0, nil)

	mb.MockMedia(imageJPG)
	mb.MockMedia(audioMP3)
	mb.MockMedia(videoMP4)
	mb.MockMedia(videoMOV)
	mb.MockMedia(filePDF)
}

func TestOutgoing(t *testing.T) {

	maxMsgLength = 160
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "LN", "2020", "US",
		map[string]any{
			"auth_token": "AccessToken",
		},
	)

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"AccessToken"}, setupMedia)
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()

	lnHandler := &handler{NewBaseHandler(courier.ChannelType("LN"), "Line")}
	req, _ := lnHandler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}
