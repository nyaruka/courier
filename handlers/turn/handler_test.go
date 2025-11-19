package turn

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/httpx"
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

var whatsappOutgoingTests = []OutgoingTestCase{
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
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Simple Message","preview_url":false}}`,
			},
		},
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
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"☺","preview_url":false}}`,
			},
		},
	},
	{
		Label:          "Audio Send",
		MsgText:        "audio caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"audio caption","preview_url":false}}`},
		},
	},
	{
		Label:          "Document Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
			},
		},
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
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
			},
		},
	},
	{
		Label:          "Image Send",
		MsgText:        "image caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"image caption"}}`,
			},
		},
	},
	{
		Label:          "Video Send",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`,
			},
		},
	},
	{
		Label:     "Template Send",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
	},
	{
		Label:          "Template Send with attachment",
		MsgText:        "templated message",
		MsgURN:         "whatsapp:250788123123",
		MsgLocale:      "eng",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/example.jpg"},
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"name": "header","type": "header/media", "variables": {"1": 0}},
				{"type": "body", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type":"image", "value":"image/jpeg:https://foo.bar/image.jpg"},
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
	},
	{
		Label:     "Template Send, no variables",
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
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"}}}`,
		}},
	},
	{
		Label:     "Template Send, buttons params",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"name": "header", "type": "header/media", "variables": {"1": 0}},
				{"name": "body", "type": "body/text", "variables": {"1": 1, "2": 2}},
				{"name": "button.0", "type": "button/quick_reply", "variables": {"1": 3}},
				{"name": "button.1", "type": "button/url", "variables": {"1": 4}}
			],
			"variables": [
				{"type": "image", "value": "image/jpeg:https://foo.bar/image.jpg"},
				{"type": "text", "value": "Ryan Lewis"},
				{"type": "text", "value": "niño"},
				{"type": "text", "value": "Sip"},
				{"type": "text", "value": "id00231"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Ryan Lewis"},{"type":"text","text":"niño"}]},{"type":"button","sub_type":"quick_reply","index":"0","parameters":[{"type":"payload","payload":"Sip"}]},{"type":"button","sub_type":"url","index":"1","parameters":[{"type":"text","text":"id00231"}]}]}}`,
		}},
	},
	{
		Label:           "Interactive Button Message Send",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "BUTTON1"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		}},
	},
	{
		Label:           "Interactive List QRs Extra Send",
		MsgText:         "Interactive List QRs Extra Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "OPTION1", Extra: "This option is the most popular"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List QRs Extra Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"OPTION1","description":"This option is the most popular"}]}]}}}`,
		}},
	},
	{
		Label:           "Interactive List QRs Extra Empty Send",
		MsgText:         "Interactive List QRs Extra Empty",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "OPTION1", Extra: ""}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive List QRs Extra Empty"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"OPTION1"}}]}}}`,
		}},
	},
	{
		Label:           "Interactive List Message Send",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "ROW1"}, {Text: "ROW2"}, {Text: "ROW3", Extra: "Third description"}, {Text: "ROW4"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3","description":"Third description"},{"id":"3","title":"ROW4"}]}]}}}`,
		}},
	},
	{
		Label:   "Interactive List Message Send, more than 10 QRs",
		MsgText: "Interactive List Msg",
		MsgURN:  "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{
			{Text: "ROW1"}, {Text: "ROW2"}, {Text: "ROW3"}, {Text: "ROW4"},
			{Text: "ROW5"}, {Text: "ROW6"}, {Text: "ROW7"}, {Text: "ROW8"},
			{Text: "ROW9"}, {Text: "ROW10"}, {Text: "ROW11"}, {Text: "ROW12"},
		},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"},{"id":"4","title":"ROW5"},{"id":"5","title":"ROW6"},{"id":"6","title":"ROW7"},{"id":"7","title":"ROW8"},{"id":"8","title":"ROW9"},{"id":"9","title":"ROW10"}]}]}}}`,
		}},
		ExpectedLogErrors: []*clogs.Error{&clogs.Error{Message: "too many quick replies WhatsApp supports only up to 10 quick replies"}},
	},
	{
		Label:           "Interactive List Message Send In Spanish",
		MsgText:         "Hola",
		MsgURN:          "whatsapp:250788123123",
		MsgLocale:       "spa",
		MsgQuickReplies: []models.QuickReply{{Text: "ROW1"}, {Text: "ROW2"}, {Text: "ROW3"}, {Text: "ROW4"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Hola"},"action":{"button":"Menú","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		}},
	},
	{
		Label:           "Interactive Button Message Send with image attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "BUTTON1"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"image","image":{"link":"https://foo.bar/image.jpg"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
	},
	{
		Label:           "Interactive Button Message Send with video attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "BUTTON1"}},
		MsgAttachments:  []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"video","video":{"link":"https://foo.bar/video.mp4"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
	},
	{
		Label:           "Interactive Button Message Send with document attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "BUTTON1"}},
		MsgAttachments:  []string{"document/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"document","document":{"link":"https://foo.bar/document.pdf","filename":"document.pdf"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
	},
	{
		Label:           "Interactive Button Message Send with audio attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "ROW1"}, {Text: "ROW2"}, {Text: "ROW3"}},
		MsgAttachments:  []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"ROW1"}},{"type":"reply","reply":{"id":"1","title":"ROW2"}},{"type":"reply","reply":{"id":"2","title":"ROW3"}}]}}}`},
		},
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Text: "ROW1"}, {Text: "ROW2"}, {Text: "ROW3"}, {Text: "ROW4"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`},
		},
	},
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
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Link Sending https://link.com","preview_url":true}}`,
			},
		},
	},
	{
		Label:   "Error Bad JSON",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(403, nil, []byte(`bad json`)),
			},
		},
		ExpectedError: courier.ErrResponseUnparseable,
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

func TestWhatsAppOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TRN", "12345_ID", "", []string{urns.WhatsApp.Prefix},
		map[string]any{models.ConfigAuthToken: "a123", "base_url": "https://example.org"})

	RunOutgoingTestCases(t, channel, newHandler(), whatsappOutgoingTests, []string{"a123"}, nil)
}
