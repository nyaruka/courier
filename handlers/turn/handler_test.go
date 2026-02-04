package turn

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"TRN",
		"250788383383",
		"RW",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			"auth_token": "a123",
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

var groupMsg = `{
	"contacts":[{
		"profile": {
			"name": "Jerry Cooney"
		},
		"wa_id": "250788123123"
	}],
  "messages": [{
    "from": "250788123123",
    "group_id": "999999999",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world, group message"
    },
    "type": "text"
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

var turnWhatsappReceiveURL = "/c/trn/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"

var testCasesTurn = []IncomingTestCase{
	{
		Label:                 "Receive Valid Message",
		URL:                   turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		URL:                  turnWhatsappReceiveURL,
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
		Label:                "Receive group message JSON, ignored",
		URL:                  turnWhatsappReceiveURL,
		Data:                 groupMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ignoring group message",
	},
	{
		Label:                "Receive invalid JSON",
		URL:                  turnWhatsappReceiveURL,
		Data:                 invalidMsg,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive invalid from",
		URL:                  turnWhatsappReceiveURL,
		Data:                 invalidFrom,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid whatsapp id",
	},
	{
		Label:                "Receive invalid timestamp",
		URL:                  turnWhatsappReceiveURL,
		Data:                 invalidTimestamp,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid timestamp",
	},

	{
		Label:                "Receive valid status",
		URL:                  turnWhatsappReceiveURL,
		Data:                 validStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "9712A34B4A8B6AD50F", Status: models.MsgStatusSent},
		},
	},
	{
		Label:                "Receive invalid JSON",
		URL:                  turnWhatsappReceiveURL,
		Data:                 "not json",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive invalid status",
		URL:                  turnWhatsappReceiveURL,
		Data:                 invalidStatus,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: `"unknown status: in_orbit"`,
	},
	{
		Label:                "Receive ignore status",
		URL:                  turnWhatsappReceiveURL,
		Data:                 ignoreStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"ignoring status: deleted"`,
	},
}

func TestIncoming(t *testing.T) {

	RunIncomingTestCases(t, testChannels, newHandler(), testCasesTurn)
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()

	waHandler := &handler{NewBaseHandler(models.ChannelType("TRN"), "WhatsApp")}
	req, _ := waHandler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer a123", req.Header.Get("Authorization"))
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Link Sending",
		MsgText: "Link Sending https://link.com",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"Link Sending https://link.com"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"to":"250788123123","type":"text","text":{"body":"Simple Message"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"text","text":{"body":"☺"}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Error Field",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "errors": [{"title":"Error Sending", "code": 232}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		}},
		ExpectedError: courier.ErrFailedWithReason("232", "Error Sending"),
	},
	{
		Label:          "Audio attachment but upload fails",
		MsgText:        "audio has no caption, sent as text",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/audio.mp3": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"to":"250788123123","type":"text","text":{"body":"audio has no caption, sent as text"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:          "Audio Send with link in text",
		MsgText:        "audio has no caption, sent as text with a https://example.com",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/audio.mp3": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(200, nil, []byte(``)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"to":"250788123123","type":"text","preview_url":true,"text":{"body":"audio has no caption, sent as text with a https://example.com"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:          "Document Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/document.pdf": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(400, nil, []byte(`{}`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Document Send, document link",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"document:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Image Send",
		MsgText:        "image caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/image.jpg": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(400, nil, []byte(`{}`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"image caption"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Video Send",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/video.mp4": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(400, nil, []byte(`{}`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"type": "body/text", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},

		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send no variables",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"}}}`,
		}},

		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template no language",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng-US",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body/text", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
			]
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},

		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Namespace",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng-US",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"namespace": "wa_template_namespace", 
			"components": [
				{"type": "body/text", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
			]
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"wa_template_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},

		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Invalid Language",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "bnt",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "body/text", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
			]
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "BUTTON1"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List QRs Extra Send",
		MsgText:         "Interactive List QRs Extra Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "OPTION1", Extra: "This option is the most popular"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List QRs Extra Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"OPTION1","description":"This option is the most popular"}]}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List QRs Extra Empty Send",
		MsgText:         "Interactive List QRs Extra Empty",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "OPTION1", Extra: ""}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive List QRs Extra Empty"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"OPTION1"}}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "ROW1"}, {Type: "text", Text: "ROW2"}, {Type: "text", Text: "ROW3", Extra: "Third description"}, {Type: "text", Text: "ROW4"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3","description":"Third description"},{"id":"3","title":"ROW4"}]}]}}}`,
		}},

		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send with attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "BUTTON1"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`},
			{Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "ROW1"}, {Type: "text", Text: "ROW2"}, {Type: "text", Text: "ROW3"}, {Type: "text", Text: "ROW4"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`},
			{Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:   "Error Channel Contact Pair limit hit",
		MsgText: "Pair limit",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(403, nil, []byte(`{ "error": {"message": "(#131056) (Business Account, Consumer Account) pair rate limit hit","code": 131056 }}`)),
			},
		},
		ExpectedError: courier.ErrConnectionThrottled,
	},
	{
		Label:   "Error Throttled",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(403, nil, []byte(`{ "error": {"message": "(#130429) Rate limit hit","code": 130429 }}`)),
			},
		},
		ExpectedError: courier.ErrConnectionThrottled,
	},
	{
		Label:   "Error",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(403, nil, []byte(`{ "error": {"message": "(#368) Temporarily blocked for policies violations","code": 368 }}`)),
			},
		},
		ExpectedError: courier.ErrFailedWithReason("368", "(#368) Temporarily blocked for policies violations"),
	},
	{
		Label:   "Error Connection",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(500, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

var mediaCacheSendTestCases = []OutgoingTestCase{
	{
		Label:          "Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/document.pdf": {
				httpx.NewMockResponse(200, nil, []byte(`media bytes`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(401, nil, []byte(`{ "errors": [{"code":1005,"title":"Access denied","details":"Invalid credentials."}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{Body: "media bytes"},
			{BodyContains: `/document.pdf`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Previous Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{BodyContains: `/document.pdf`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Media Upload OK",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/video.mp4": {
				httpx.NewMockResponse(200, nil, []byte(`media bytes`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "36c484d1-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{Body: "media bytes"},
			{Body: `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Cached Media",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"video","video":{"id":"36c484d1-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Document Upload OK",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/document2.pdf": {
				httpx.NewMockResponse(200, nil, []byte(`media bytes`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "25c484d1-1283-4b94-988d-7276bdec4ef3"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{Body: "media bytes"},
			{Body: `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Cached Document",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document2.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"document","document":{"id":"25c484d1-1283-4b94-988d-7276bdec4ef3","caption":"document caption","filename":"document2.pdf"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
}

func TestWhatsAppOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TRN", "12345_ID", "", []string{urns.WhatsApp.Prefix},
		map[string]any{models.ConfigAuthToken: "a123", "base_url": "https://example.org", "fb_namespace": "waba_namespace"})

	RunOutgoingTestCases(t, channel, newHandler(), defaultSendTestCases, []string{"a123"}, nil)
	failedMediaCache.Flush()
	RunOutgoingTestCases(t, channel, newHandler(), mediaCacheSendTestCases, []string{"a123"}, nil)
	failedMediaCache.Flush()
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
