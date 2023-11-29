package whatsapp_legacy

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
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"WA",
		"250788383383",
		"RW",
		map[string]any{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"D3",
		"250788383383",
		"RW",
		map[string]any{
			"auth_token": "the-auth-token",
			"base_url":   "https://foo.bar/",
		}),
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"TXW",
		"250788383383",
		"RW",
		map[string]any{
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
			"id": "41",
			"mime_type": "text/plain",
			"sha256": "the-sha-signature",
			"caption": "the caption",
			"filename": "filename.type"
		}
	}]
}`

var documentMsgMissingFile = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "document",
		"document": {
			"mime_type": "text/plain",
			"sha256": "the-sha-signature",
			"caption": "the caption",
			"filename": "filename.type",
			"status": "undownloaded"
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
    "status": "sent",
    "timestamp": "1518694700"
  }]
}
`
var invalidStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "status": "in_orbit",
    "timestamp": "1518694700"
  }]
}
`
var ignoreStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
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

var waTestCases = []IncomingTestCase{
	{
		Label:                 "Receive Valid Message",
		URL:                   waReceiveURL,
		Data:                  helloMsg,
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  `"type":"msg"`,
		ExpectedContactName:   Sp("Jerry Cooney"),
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "whatsapp:250788123123",
		ExpectedExternalID:    "41",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
	},
	{
		Label:                "Receive duplicate valid message",
		URL:                  waReceiveURL,
		Data:                 duplicateMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("hello world"),
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid audio message",
		URL:                  waReceiveURL,
		Data:                 audioMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://foo.bar/v1/media/41"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid button message",
		URL:                  waReceiveURL,
		Data:                 buttonMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("BUTTON1"),
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid document message",
		URL:                  waReceiveURL,
		Data:                 documentMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("the caption"),
		ExpectedAttachments:  []string{"https://foo.bar/v1/media/41"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid image message",
		URL:                  waReceiveURL,
		Data:                 imageMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("the caption"),
		ExpectedAttachments:  []string{"https://foo.bar/v1/media/41"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid interactive button message",
		URL:                  waReceiveURL,
		Data:                 interactiveButtonMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("BUTTON1"),
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid interactive list message",
		URL:                  waReceiveURL,
		Data:                 interactiveListMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("ROW1"),
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid location message",
		URL:                  waReceiveURL,
		Data:                 locationMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:0.000000,1.000000"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid video message",
		URL:                  waReceiveURL,
		Data:                 videoMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://foo.bar/v1/media/41"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive valid voice message",
		URL:                  waReceiveURL,
		Data:                 voiceMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://foo.bar/v1/media/41"},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive document message with missing file",
		URL:                  waReceiveURL,
		Data:                 documentMsgMissingFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("the caption"),
		ExpectedAttachments:  []string{},
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive invalid JSON",
		URL:                  waReceiveURL,
		Data:                 invalidMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive invalid from",
		URL:                  waReceiveURL,
		Data:                 invalidFrom,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid whatsapp id",
	},
	{
		Label:                "Receive invalid timestamp",
		URL:                  waReceiveURL,
		Data:                 invalidTimestamp,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid timestamp",
	},

	{
		Label:                "Receive valid status",
		URL:                  waReceiveURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "9712A34B4A8B6AD50F", Status: courier.MsgStatusSent},
		},
	},
	{
		Label:                "Receive invalid JSON",
		URL:                  waReceiveURL,
		Data:                 "not json",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive invalid status",
		URL:                  waReceiveURL,
		Data:                 invalidStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"unknown status: in_orbit"`,
	},
	{
		Label:                "Receive ignore status",
		URL:                  waReceiveURL,
		Data:                 ignoreStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"ignoring status: deleted"`,
	},
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()

	waHandler := &handler{NewBaseHandler(courier.ChannelType("WA"), "WhatsApp")}
	req, _ := waHandler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))

	d3Handler := &handler{NewBaseHandler(courier.ChannelType("D3"), "360Dialog")}
	req, _ = d3Handler.BuildAttachmentRequest(context.Background(), mb, testChannels[1], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "the-auth-token", req.Header.Get("D360-API-KEY"))

	txHandler := &handler{NewBaseHandler(courier.ChannelType("TXW"), "TextIt")}
	req, _ = txHandler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer the-auth-token", req.Header.Get("Authorization"))
}

func replaceTestcaseURLs(tcs []IncomingTestCase, url string) []IncomingTestCase {
	replaced := make([]IncomingTestCase, len(tcs))
	for i, tc := range tcs {
		tc.URL = url
		replaced[i] = tc
	}
	return replaced
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), waTestCases)
	RunIncomingTestCases(t, testChannels, newWAHandler(courier.ChannelType("D3"), "360Dialog"), replaceTestcaseURLs(waTestCases, d3ReceiveURL))
	RunIncomingTestCases(t, testChannels, newWAHandler(courier.ChannelType("TXW"), "TextIt"), replaceTestcaseURLs(waTestCases, txReceiveURL))
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), waTestCases)
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("D3"), "360Dialog"), replaceTestcaseURLs(waTestCases, d3ReceiveURL))
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("TXW"), "TextIt"), replaceTestcaseURLs(waTestCases, txReceiveURL))
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	retryParam = "retry"
	c.(*test.MockChannel).SetConfig("base_url", s.URL)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Link Sending",
		MsgText:             "Link Sending https://link.com",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"Link Sending https://link.com"}}`,
		ExpectedRequestPath: "/v1/messages",
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Rate Limit Engaged",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "errors": [{ "title": "Too many requests" }] }`,
		MockResponseStatus:  429,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "No Message ID",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Error Field",
		MsgText:             "Error",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "errors": [{"title":"Error Sending"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		ExpectedMsgStatus:   "E",
		SendPrep:            setSendURL,
	},
	{
		Label:          "Audio Send",
		MsgText:        "audio has no caption, sent as text",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"audio has no caption, sent as text"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Audio Send with link in text",
		MsgText:        "audio has no caption, sent as text with a https://example.com",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"audio has no caption, sent as text with a https://example.com"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Document Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Image Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"document caption"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Video Send",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Template Send",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Send no variables",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": []}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Country Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng-US",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Namespace",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng-US",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "namespace": "wa_template_namespace", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"wa_template_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Invalid Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "bnt",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:   "WhatsApp Contact Error",
		MsgText: "contact status error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"contact status error"}}`,
			}: httpx.NewMockResponse(404, nil, []byte(`{"errors": [{"code":1006,"title":"Resource not found","details":"unknown contact"}]}`)),
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"contacts":[{"input":"+250788123123","status":"invalid"}]}`)),
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check",
		MsgText: "try again",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(404, nil, []byte(`{"errors": [{"code": 1006, "title": "Resource not found", "details": "unknown contact"}]}`)),
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"contacts": [{"input": "+250788123123", "status": "valid", "wa_id": "250788123123"}]}`)),
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{"messages": [{"id": "157b5e14568e8"}]}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check",
		MsgText: "try again",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(404, nil, []byte(`{"errors": [{"code": 1006, "title": "Resource not found", "details": "Could not retrieve phone number from contact store"}]}`)),
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+250788123123"],"force_check":true}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"contacts": [{"input": "+250788123123", "status": "valid", "wa_id": "250788123123"}]}`)),
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"250788123123","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{"messages": [{"id": "157b5e14568e8"}]}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:   "Try Messaging Again After WhatsApp Contact Check With Returned WhatsApp ID",
		MsgText: "try again",
		MsgURN:  "whatsapp:5582999887766",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"5582999887766","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(404, nil, []byte(`{"errors": [{"code": 1006, "title": "Resource not found", "details": "unknown contact"}]}`)),
			{
				Method: "POST",
				Path:   "/v1/contacts",
				Body:   `{"blocking":"wait","contacts":["+5582999887766"],"force_check":true}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"contacts": [{"input": "+5582999887766", "status": "valid", "wa_id": "558299887766"}]}`)),
			{
				Method:   "POST",
				Path:     "/v1/messages",
				RawQuery: "retry=1",
				Body:     `{"to":"558299887766","type":"text","text":{"body":"try again"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{"messages": [{"id": "157b5e14568e8"}]}`)),
		},
		ExpectedMsgStatus:  "W",
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
		ExpectedMsgStatus:   "W",
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL},
	{
		Label:           "Interactive Button Message Send with attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		ExpectedNewURN:      "whatsapp:551187654321",
		SendPrep:            setSendURL,
	},
}

var mediaCacheSendTestCases = []OutgoingTestCase{
	{
		Label:          "Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: httpx.NewMockResponse(401, nil, []byte(`{ "errors": [{"code":1005,"title":"Access denied","details":"Invalid credentials."}] }`)),
			{
				Method:       "POST",
				Path:         "/v1/messages",
				BodyContains: `/document.pdf`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Previous Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/v1/messages",
				BodyContains: `/document.pdf`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Media Upload OK",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "36c484d1-1283-4b94-988d-7276bdec4de2"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Cached Media",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Document Upload OK",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/media",
				Body:   "media bytes",
			}: httpx.NewMockResponse(200, nil, []byte(`{ "media" : [{"id": "25c484d1-1283-4b94-988d-7276bdec4ef3"}] }`)),
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Cached Document",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/v1/messages",
				Body:   `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []OutgoingTestCase) []OutgoingTestCase {
	casesWithMockedUrls := make([]OutgoingTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.MsgAttachments {
			mockedCase.MsgAttachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WA", "250788383383", "US",
		map[string]any{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	var d3Channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3", "250788383383", "US",
		map[string]any{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	var txwChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TXW", "250788383383", "US",
		map[string]any{
			"auth_token":   "token123",
			"base_url":     "https://foo.bar/",
			"fb_namespace": "waba_namespace",
			"version":      "v2.35.2",
		})

	RunOutgoingTestCases(t, defaultChannel, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), defaultSendTestCases, []string{"token123"}, nil)
	RunOutgoingTestCases(t, d3Channel, newWAHandler(courier.ChannelType("D3"), "360Dialog"), defaultSendTestCases, []string{"token123"}, nil)
	RunOutgoingTestCases(t, txwChannel, newWAHandler(courier.ChannelType("TXW"), "TextIt"), defaultSendTestCases, []string{"token123"}, nil)

	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media bytes"))
	}))
	defer mediaServer.Close()
	mediaCacheSendTestCases := mockAttachmentURLs(mediaServer, mediaCacheSendTestCases)

	RunOutgoingTestCases(t, defaultChannel, newWAHandler(courier.ChannelType("WA"), "WhatsApp"), mediaCacheSendTestCases, []string{"token123"}, nil)
}

func TestGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, "en", getSupportedLanguage(i18n.NilLocale))
	assert.Equal(t, "en", getSupportedLanguage(i18n.Locale("eng")))
	assert.Equal(t, "en_US", getSupportedLanguage(i18n.Locale("eng-US")))
	assert.Equal(t, "pt_PT", getSupportedLanguage(i18n.Locale("por")))
	assert.Equal(t, "pt_PT", getSupportedLanguage(i18n.Locale("por-PT")))
	assert.Equal(t, "pt_BR", getSupportedLanguage(i18n.Locale("por-BR")))
	assert.Equal(t, "fil", getSupportedLanguage(i18n.Locale("fil")))
	assert.Equal(t, "fr", getSupportedLanguage(i18n.Locale("fra-CA")))
	assert.Equal(t, "en", getSupportedLanguage(i18n.Locale("run")))
}
