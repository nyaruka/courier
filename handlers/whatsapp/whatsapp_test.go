package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"WA",
		"250788383383",
		"RW",
		map[string]interface{}{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"D3",
		"250788383383",
		"RW",
		map[string]interface{}{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"TXW",
		"250788383383",
		"RW",
		map[string]interface{}{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
}

var helloMsg = `{
	"contacts":[{
		"profile": {
			"name": "Jerry Cooney"
		},
		"wa_id": "250788123123"
	}],
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

var buttonMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "button",
		"button": {
			"payload": null,
			"text": "BUTTON1"
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
			"caption": "the caption",
			"filename": "filename.type"
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

var interactiveButtonMsg = `{
  "messages": [{
		"from": "250788123123",
		"id": "41",
		"interactive": {
			"button_reply": {
				"id": "0",
				"title": "BUTTON1"
			},
			"type": "button_reply"
		},
		"timestamp": "1454119029",
		"type": "interactive"
	}]
}`

var interactiveListMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"interactive": {
			"list_reply": {
				"id": "0",
				"title": "ROW1"
			},
			"type": "list_reply"
		},
		"timestamp": "1454119029",
		"type": "interactive"
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

var (
	waReceiveURL = "/c/wa/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
	d3ReceiveURL = "/c/d3/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
	txReceiveURL = "/c/txw/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
)

var waTestCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: waReceiveURL, Data: helloMsg, Status: 200, Response: `"type":"msg"`,
		Name: Sp("Jerry Cooney"), Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Duplicate Valid Message", URL: waReceiveURL, Data: duplicateMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("hello world"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Audio Message", URL: waReceiveURL, Data: audioMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Button Message", URL: waReceiveURL, Data: buttonMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("BUTTON1"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Document Message", URL: waReceiveURL, Data: documentMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("the caption"), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Image Message", URL: waReceiveURL, Data: imageMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("the caption"), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Interactive Button Message", URL: waReceiveURL, Data: interactiveButtonMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("BUTTON1"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Interactive List Message", URL: waReceiveURL, Data: interactiveListMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp("ROW1"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Location Message", URL: waReceiveURL, Data: locationMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("geo:0.000000,1.000000"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Video Message", URL: waReceiveURL, Data: videoMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Valid Voice Message", URL: waReceiveURL, Data: voiceMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("https://foo.bar/v1/media/41"), URN: Sp("whatsapp:250788123123"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},
	{Label: "Receive Invalid JSON", URL: waReceiveURL, Data: invalidMsg, Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid From", URL: waReceiveURL, Data: invalidFrom, Status: 400, Response: "invalid whatsapp id"},
	{Label: "Receive Invalid Timestamp", URL: waReceiveURL, Data: invalidTimestamp, Status: 400, Response: "invalid timestamp"},

	{Label: "Receive Valid Status", URL: waReceiveURL, Data: validStatus, Status: 200, Response: `"type":"status"`,
		MsgStatus: Sp("S"), ExternalID: Sp("9712A34B4A8B6AD50F")},
	{Label: "Receive Invalid JSON", URL: waReceiveURL, Data: "not json", Status: 400, Response: "unable to parse"},
	{Label: "Receive Invalid Status", URL: waReceiveURL, Data: invalidStatus, Status: 400, Response: `"unknown status: in_orbit"`},
	{Label: "Receive Ignore Status", URL: waReceiveURL, Data: ignoreStatus, Status: 200, Response: `"ignoring status: deleted"`},
}

func TestBuildMediaRequest(t *testing.T) {
	mb := test.NewMockBackend()

	waHandler := &handler{NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
	req, _ := waHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))

	d3Handler := &handler{NewBaseHandler(courier.ChannelType("D3"), "360Dialog")}
	req, _ = d3Handler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[1], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "the-auth-token", req.Header.Get("D360-API-KEY"))

	txHandler := &handler{NewBaseHandler(courier.ChannelType("TXW"), "TextIt")}
	req, _ = txHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}

func replaceTestcaseURLs(tcs []ChannelHandleTestCase, url string) []ChannelHandleTestCase {
	replaced := make([]ChannelHandleTestCase, len(tcs))
	for i, tc := range tcs {
		tc.URL = url
		replaced[i] = tc
	}
	return replaced
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), waTestCases)
	RunChannelTestCases(t, testChannels, newWAHandler(courier.ChannelType("D3"), "360Dialog"), replaceTestcaseURLs(waTestCases, d3ReceiveURL))
	RunChannelTestCases(t, testChannels, newWAHandler(courier.ChannelType("TXW"), "TextIt"), replaceTestcaseURLs(waTestCases, txReceiveURL))
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), waTestCases)
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("D3"), "360Dialog"), replaceTestcaseURLs(waTestCases, d3ReceiveURL))
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("TXW"), "TextIt"), replaceTestcaseURLs(waTestCases, txReceiveURL))
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	retryParam = "retry"
	c.(*test.MockChannel).SetConfig("base_url", s.URL)
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Link Sending",
		MsgText:             "Link Sending https://link.com",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"Link Sending https://link.com"}}`,
		ExpectedRequestPath: "/v1/messages",
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Simple Message"}}`,
		ExpectedRequestPath: "/v1/messages",
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"☺"}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "errors": [{ "title": "Error Sending" }] }`,
		MockResponseStatus:  403,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedStatus:      "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Rate Limit Engaged",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "errors": [{ "title": "Too many requests" }] }`,
		MockResponseStatus:  429,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedStatus:      "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "No Message ID",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedStatus:      "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Field",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "errors": [{"title":"Error Sending"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedStatus:      "E",
		SendPrep:            setSendURL,
	},
	{
		Label:          "Audio Send",
		MsgText:        "audio has no caption, sent as text",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"audio has no caption, sent as text"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Audio Send with link in text",
		MsgText:        "audio has no caption, sent as text with a https://example.com",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"audio has no caption, sent as text with a https://example.com"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Document Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Image Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"document caption"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Video Send",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Template Send",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Country Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "country": "US", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Namespace",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "namespace": "wa_template_namespace", "language": "eng", "country": "US", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"wa_template_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:         "Template Invalid Language",
		MsgText:       "templated message",
		MsgURN:        "whatsapp:250788123123",
		ExpectedError: `unable to decode template: {"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}} for channel: 8eb23e93-5ecb-45ba-b726-3b064e0c56ab: unable to find mapping for language: bnt`,
		MsgMetadata:   json.RawMessage(`{"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}}`),
	},
	{
		Label:   "WhatsApp Contact Error",
		MsgText: "contact status error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"contact status error"}}`,
			}: {
				Status: 404,
				Body:   `{"errors": [{"code":1006,"title":"Resource not found","details":"unknown contact"}]}`,
			},
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: {
				Status: 200,
				Body:   `{"contacts":[{"input":"+250788123123","status":"invalid"}]}`,
			},
		},
		ExpectedStatus: "E",
		SendPrep:       setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check",
		MsgText: "try again",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 404,
				Body:   `{"errors": [{"code": 1006, "title": "Resource not found", "details": "unknown contact"}]}`,
			},
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: {
				Status: 200,
				Body:   `{"contacts": [{"input": "+250788123123", "status": "valid", "wa_id": "250788123123"}]}`,
			},
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 201,
				Body:   `{"messages": [{"id": "157b5e14568e8"}]}`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check",
		MsgText: "try again",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 404,
				Body:   `{"errors": [{"code": 1006, "title": "Resource not found", "details": "Could not retrieve phone number from contact store"}]}`,
			},
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: {
				Status: 200,
				Body:   `{"contacts": [{"input": "+250788123123", "status": "valid", "wa_id": "250788123123"}]}`,
			},
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 201,
				Body:   `{"messages": [{"id": "157b5e14568e8"}]}`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check With Returned WhatsApp ID",
		MsgText: "try again",
		MsgURN:  "whatsapp:5582999887766",
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"5582999887766","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 404,
				Body:   `{"errors": [{"code": 1006, "title": "Resource not found", "details": "unknown contact"}]}`,
			},
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+5582999887766"],"force_check":true}`,
			}: {
				Status: 200,
				Body:   `{"contacts": [{"input": "+5582999887766", "status": "valid", "wa_id": "558299887766"}]}`,
			},
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"558299887766","type":"text","text":{"body":"try again"}}`,
			}: {
				Status: 201,
				Body:   `{"messages": [{"id": "157b5e14568e8"}]}`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Interactive Button Message Send",
		MsgText:             "Interactive Button Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"BUTTON1"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL},
	{
		Label:               "Interactive List Message Send",
		MsgText:             "Interactive List Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL},
	{
		Label:           "Interactive Button Message Send with attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Update URN with wa_id returned",
		MsgText:             "Simple Message",
		MsgURN:              "whatsapp:5511987654321",
		MockResponseBody:    `{ "contacts":[{"input":"5511987654321","wa_id":"551187654321"}], "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"5511987654321","type":"text","text":{"body":"Simple Message"}}`,
		ExpectedRequestPath: "/v1/messages",
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		ExpectedNewURN:      "whatsapp:551187654321",
		SendPrep:            setSendURL,
	},
}

var mediaCacheSendTestCases = []ChannelSendTestCase{
	{
		Label:          "Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: {
				Status: 401,
				Body:   `{ "errors": [{"code":1005,"title":"Access denied","details":"Invalid credentials."}] }`,
			},
			{
				Method:       "POST",
				Path:         "/v1/messages",
				BodyContains: `/document.pdf`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Previous Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method:       "POST",
				Path:         "/v1/messages",
				BodyContains: `/document.pdf`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Media Upload OK",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: {
				Status: 200,
				Body:   `{ "media" : [{"id": "36c484d1-1283-4b94-988d-7276bdec4de2"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Cached Media",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Document Upload OK",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: {
				Status: 200,
				Body:   `{ "media" : [{"id": "25c484d1-1283-4b94-988d-7276bdec4ef3"}] }`,
			},
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Cached Document",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[MockedRequest]MockedResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`,
			}: {
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		ExpectedStatus:     "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
}

var hsmSupportSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Template Send",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"hsm","hsm":{"namespace":"waba_namespace","element_name":"revive_issue","language":{"policy":"deterministic","code":"en"},"localizable_params":[{"default":"Chef"},{"default":"tomorrow"}]}}`,
		ExpectedStatus:      "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.MsgAttachments {
			mockedCase.MsgAttachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestSending(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]interface{}{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	var hsmSupportChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]interface{}{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"hsm_support":  true,
			"version":      "v2.35.2",
		})

	var d3Channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3", "250788383383", "US",
		map[string]interface{}{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	var txwChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TXW", "250788383383", "US",
		map[string]interface{}{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	RunChannelSendTestCases(t, defaultChannel, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, hsmSupportChannel, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), hsmSupportSendTestCases, nil)
	RunChannelSendTestCases(t, d3Channel, newWAHandler(courier.ChannelType("D3"), "360Dialog"), defaultSendTestCases, nil)
	RunChannelSendTestCases(t, txwChannel, newWAHandler(courier.ChannelType("TXW"), "TextIt"), defaultSendTestCases, nil)

	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media bytes"))
	}))
	defer mediaServer.Close()
	mediaCacheSendTestCases := mockAttachmentURLs(mediaServer, mediaCacheSendTestCases)

	RunChannelSendTestCases(t, defaultChannel, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), mediaCacheSendTestCases, nil)
}
