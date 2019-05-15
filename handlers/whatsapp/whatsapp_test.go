package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"WA",
		"250788383383",
		"RW",
		map[string]interface{}{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
}

var helloMsg = `{
  "messages": [{
    "from": "250788123123",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var duplicateMsg = `{
	"messages": [{
	  "from": "250788123123",
	  "id": "41",
	  "timestamp": "1454119029",
	  "text": {
		"body": "hello world"
	  },
	  "type": "text"
	}, {
	  "from": "250788123123",
	  "id": "41",
	  "timestamp": "1454119029",
	  "text": {
		"body": "hello world"
	  },
	  "type": "text"
	}]
  }`

var audioMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "audio",
		"audio": {
			"file": "/path/to/v1/media/41",
			"id": "41",
			"link": "https://example.org/v1/media/41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature"
		}
	}]
}`

var documentMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "document",
		"document": {
			"file": "/path/to/v1/media/41",
			"id": "41",
			"link": "https://example.org/v1/media/41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature",
			"caption": "the caption"
		}
	}]
}`

var imageMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "image",
		"image": {
			"file": "/path/to/v1/media/41",
			"id": "41",
			"link": "https://example.org/v1/media/41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature",
			"caption": "the caption"
		}
	}]
}`

var locationMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "location",
		"location": {
			"address": "some address",
			"latitude": 0.00,
			"longitude": 1.00,
			"name": "some name",
			"url": "https://example.org/"
		}
	}]
}`

var videoMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "video",
		"video": {
			"file": "/path/to/v1/media/41",
			"id": "41",
			"link": "https://example.org/v1/media/41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature"
		}
	}]
}`

var voiceMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "voice",
		"voice": {
			"file": "/path/to/v1/media/41",
			"id": "41",
			"link": "https://example.org/v1/media/41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature"
		}
	}]
}`

var invalidFrom = `{
  "messages": [{
    "from": "notnumber",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var invalidTimestamp = `{
  "messages": [{
    "from": "notnumber",
    "id": "41",
    "timestamp": "asdf",
    "text": {
      "body": "hello world"
    },
    "type": "text"
  }]
}`

var invalidMsg = `not json`

var validStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "recipient_id": "16315555555",
    "status": "sent",
    "timestamp": "1518694700"
  }]
}
`

var invalidStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "recipient_id": "16315555555",
    "status": "in_orbit",
    "timestamp": "1518694700"
  }]
}
`

var ignoreStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "recipient_id": "16315555555",
    "status": "deleted",
    "timestamp": "1518694700"
  }]
}
`

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Duplicate Valid Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: duplicateMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Audio Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: audioMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Document Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: documentMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("the caption"), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Image Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: imageMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("the caption"), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Location Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: locationMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("geo:0.000000,1.000000"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Video Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: videoMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Voice Message", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: voiceMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidMsg, Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid From", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidFrom, Status: 400, Response: "invalid whatsapp id"},
	{Label: "Receive Invalid Timestamp", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTimestamp, Status: 400, Response: "invalid timestamp"},

	{Label: "Receive Valid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: validStatus, Status: 200, Response: `"type":"status"`,
		MsgStatus: Sp("S"), ExternalID: Sp("9712A34B4A8B6AD50F")},
	{Label: "Receive Invalid JSON", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: "not json", Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidStatus, Status: 400, Response: `"unknown status: in_orbit"`},
	{Label: "Receive Ignore Status", URL: "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: ignoreStatus, Status: 200, Response: `"ignoring status: deleted"`},
}

func TestBuildMediaRequest(t *testing.T) {
	mb := courier.NewMockBackend()

	handler := &handler{NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
	req, _ := handler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig("base_url", s.URL)
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))
	for i, testCase := range testCases {
		mockedCase := testCase
		for j, attachment := range testCase.Attachments {
			parts := strings.SplitN(attachment, ":", 2)
			mimeType := parts[0]
			urlString := parts[1]
			parsedURL, _ := url.Parse(urlString)
			mockedCase.Attachments[j] = fmt.Sprintf("%s:%s%s", mimeType, mediaServer.URL, parsedURL.Path)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"☺"}}`,
		SendPrep:    setSendURL},
	{Label: "Error",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "errors": [{ "title": "Error Sending" }] }`, ResponseStatus: 403,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "No Message ID",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "messages": [] }`, ResponseStatus: 200,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "Error Field",
		Text: "Error", URN: "whatsapp:250788123123",
		Status:       "E",
		ResponseBody: `{ "errors": [{"title":"Error Sending"}] }`, ResponseStatus: 200,
		RequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		SendPrep:    setSendURL},
	{Label: "Audio Send",
		Text:   "audio has no caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media body",
			}: MockedResponse{
				Status: 201,
				Body:   `{"media": [{"id": "media-id"}]}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"audio","audio":{"id":"media-id"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Document Send",
		Text:   "document caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media body",
			}: MockedResponse{
				Status: 201,
				Body:   `{"media": [{"id": "media-id"}]}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"id":"media-id","caption":"document caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Image Send",
		Text:   "document caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media body",
			}: MockedResponse{
				Status: 201,
				Body:   `{"media": [{"id": "media-id"}]}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"id":"media-id","caption":"document caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Video Send",
		Text:   "video caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media body",
			}: MockedResponse{
				Status: 201,
				Body:   `{"media": [{"id": "media-id"}]}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"id":"media-id","caption":"video caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Template Send",
		Text:   "templated message",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Metadata:     json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "variables": ["Chef", "tomorrow"]}}`),
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 200,
		RequestBody: `{"to":"250788123123","type":"hsm","hsm":{"namespace":"waba_namespace","element_name":"revive_issue","language":{"policy":"deterministic","code":"en"},"localizable_params":[{"default":"Chef"},{"default":"tomorrow"}]}}`,
		SendPrep:    setSendURL,
	},
	{Label: "Template Invalid Language",
		Text: "templated message", URN: "whatsapp:250788123123",
		Error:    `unable to decode template: {"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}} for channel: 8eb23e93-5ecb-45ba-b726-3b064e0c56ab: unable to find mapping for language: bnt`,
		Metadata: json.RawMessage(`{"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}}`),
	},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]interface{}{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
		})

	// fake media server that just replies with 200 and "media body" for content
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media body"))
	}))

	attachmentMockedSendTestCase := mockAttachmentURLs(mediaServer, defaultSendTestCases)
	RunChannelSendTestCases(t, defaultChannel, newHandler(), attachmentMockedSendTestCase, nil)
}
