package turn

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
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

var helloMsgWithValidBSUID = `{
  "contacts":[{
    "profile": {
      "name": "Jerry Cooney"
    },
    "wa_id": "250788123123"
  }],
  "messages": [{
    "from": "250788123123",
    "from_bsuid": "US.1234",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
   }]
}`

var helloMsgFromBSUID = `{
  "contacts":[{
    "profile": {
      "name": "Jerry Cooney"
    },
    "user_id": "US.1234"
  }],
  "messages": [{
    "from_bsuid": "US.1234",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
   }]
}`

var helloMsgBSUIDInFrom = `{
  "contacts":[{
    "profile": {
      "name": "Jerry Cooney"
    },
    "wa_id": "US.1234"
  }],
  "messages": [{
    "from": "US.1234",
    "from_bsuid": "US.1234",
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
   }]
}`

var helloMsgNoFromOrBSUID = `{
  "contacts":[{
    "profile": {
      "name": "Jerry Cooney"
    }
  }],
  "messages": [{
    "id": "41",
    "timestamp": "1454119029",
    "text": {
      "body": "hello world"
    },
    "type": "text"
   }]
}`

var helloMsgWithInvalidBSUID = `{
  "contacts":[{
    "profile": {
      "name": "Jerry Cooney"
    },
    "wa_id": "250788123123"
  }],
  "messages": [{
    "from": "250788123123",
    "from_bsuid": "foo_bar",
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
			"sha256": "the-sha-signature",
			"caption": "the caption"
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

var nfmReplyMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"interactive": {
			"nfm_reply": {
				"name": "flow",
				"body": "Sent",
				"response_json": "{\"flow_token\": \"fl0w+t0k3n\", \"age\": \"32\"}"
			},
			"type": "nfm_reply"
		},
		"timestamp": "1454119029",
		"type": "interactive"
	}]
}`

var nfmReplyMsgNonObject = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"interactive": {
			"nfm_reply": {
				"name": "flow",
				"body": "Sent",
				"response_json": "[1, 2]"
			},
			"type": "nfm_reply"
		},
		"timestamp": "1454119029",
		"type": "interactive"
	}]
}`

var unsupportedTypeMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "order"
	}]
}`

var errorMsg = `{
	"messages": [{
		"from": "250788123123",
		"id": "41",
		"timestamp": "1454119029",
		"type": "unsupported",
		"errors": [{"code": 131051, "title": "Unsupported message type"}]
	}]
}`

var errorStatus = `
{
  "statuses": [{
    "id": "9712A34B4A8B6AD50F",
    "status": "failed",
    "timestamp": "1518694700",
    "errors": [{"code": 131014, "title": "Request for url https://URL.jpg failed with error: 404 (Not Found)"}]
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
		NoInvalidChannelCheck: true,
	},
	{
		Label:                 "Receive Message with valid bsuid",
		URL:                   turnWhatsappReceiveURL,
		Data:                  helloMsgWithValidBSUID,
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  `"type":"msg"`,
		ExpectedContactName:   Sp("Jerry Cooney"),
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "whatsapp:250788123123",
		ExpectedExternalID:    "41",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		ExpectedNewURN:        &models.NewURNSpec{Value: "whatsapp:US.1234", Action: models.NewURNAppend},
		NoInvalidChannelCheck: true,
	},
	{
		Label:                 "Receive Message from BSUID with no phone",
		URL:                   turnWhatsappReceiveURL,
		Data:                  helloMsgFromBSUID,
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  `"type":"msg"`,
		ExpectedContactName:   Sp("Jerry Cooney"),
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "whatsapp:US.1234",
		ExpectedExternalID:    "41",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		NoInvalidChannelCheck: true,
	},
	{
		Label:                 "Receive Message with BSUID in from",
		URL:                   turnWhatsappReceiveURL,
		Data:                  helloMsgBSUIDInFrom,
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  `"type":"msg"`,
		ExpectedContactName:   Sp("Jerry Cooney"),
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "whatsapp:US.1234",
		ExpectedExternalID:    "41",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		NoInvalidChannelCheck: true,
	},
	{
		Label:                "Receive Message with no from or from_bsuid",
		URL:                  turnWhatsappReceiveURL,
		Data:                 helloMsgNoFromOrBSUID,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "invalid whatsapp id",
	},
	{
		Label:                 "Receive Message with invalid bsuid, no new URN added",
		URL:                   turnWhatsappReceiveURL,
		Data:                  helloMsgWithInvalidBSUID,
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  `"type":"msg"`,
		ExpectedContactName:   Sp("Jerry Cooney"),
		ExpectedMsgText:       Sp("hello world"),
		ExpectedURN:           "whatsapp:250788123123",
		ExpectedExternalID:    "41",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		ExpectedNewURN:        nil,
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
		Label:                "Receive valid interactive flow reply message",
		URL:                  turnWhatsappReceiveURL,
		Data:                 nfmReplyMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("Sent"),
		ExpectedPayload:      `{"flow_token": "fl0w+t0k3n", "age": "32"}`,
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive interactive flow reply message with non-object response JSON",
		URL:                  turnWhatsappReceiveURL,
		Data:                 nfmReplyMsgNonObject,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp("Sent"),
		ExpectedURN:          "whatsapp:250788123123",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive unsupported message type, no message written",
		URL:                  turnWhatsappReceiveURL,
		Data:                 unsupportedTypeMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Events Handled",
	},
	{
		Label:                "Receive message with errors, logged and no message written",
		URL:                  turnWhatsappReceiveURL,
		Data:                 errorMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Events Handled",
		ExpectedErrors:       []*svclogs.Error{models.ErrorExternal("131051", "Unsupported message type")},
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
		ExpectedMsgText:      Sp("the caption"),
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
		Label:                "Receive failed status with error message",
		URL:                  turnWhatsappReceiveURL,
		Data:                 errorStatus,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "9712A34B4A8B6AD50F", Status: models.MsgStatusFailed},
		},
		ExpectedErrors: []*svclogs.Error{
			models.ErrorExternal("131014", "Request for url https://URL.jpg failed with error: 404 (Not Found)"),
		},
	},
	{
		Label:                "Receive invalid status",
		URL:                  turnWhatsappReceiveURL,
		Data:                 invalidStatus,
		ExpectedRespStatus:   200,
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
	waHandler := &handler{NewBaseHandler(models.ChannelType("TRN"), "WhatsApp")}
	req, _ := waHandler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://example.org/v1/media/41", nil)
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
		Label:   "Plain Send with BSUID",
		MsgText: "Simple Message",
		MsgURN:  "bsuid:US.1234",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"recipient":"US.1234","type":"text","text":{"body":"Simple Message"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Plain Send with BSUID as whatsapp URN",
		MsgText: "Simple Message",
		MsgURN:  "whatsapp:US.1234",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/v1/messages",
				Body: `{"recipient":"US.1234","type":"text","text":{"body":"Simple Message"}}`,
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
		ExpectedError: channels.ErrFailedWithReason("232", "Error Sending"),
	},
	{
		Label:   "Error Field with details",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(400, nil, []byte(`{ "errors": [{"code": -1, "title": "Bad Request", "details": "Could not be parsed, invalid key"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		}},
		ExpectedError: channels.ErrFailedWithReason("-1", "Bad Request: Could not be parsed, invalid key"),
	},
	{
		Label:   "Error Field Retryable",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "errors": [{"title":"Media upload error", "code": 131053}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"text","text":{"body":"Error"}}`,
		}},
		ExpectedError: channels.ErrRetryableWithReason("131053", "Media upload error"),
	},
	{
		// media messages can't carry a link, so a failed upload has to error rather than fall back
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
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
		},
		ExpectedError: channels.ErrRetryableWithReason("media_upload_failed", "unable to upload media to WhatsApp"),
	},
	{
		Label:          "Audio Send with link in text",
		MsgText:        "audio has no caption, sent as text with a https://example.com",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio2.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/audio2.mp3": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "8a1b0c3d-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"audio","audio":{"id":"8a1b0c3d-1283-4b94-988d-7276bdec4de2"}}`},
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
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "1b2c3d4e-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"document","document":{"id":"1b2c3d4e-1283-4b94-988d-7276bdec4de2","caption":"document caption","filename":"document.pdf"}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		// the filename still comes from the attachment URL even though the payload references media by id
		Label:          "Document Send, document link",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"document:https://foo.bar/document3.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/document3.pdf": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "2c3d4e5f-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"document","document":{"id":"2c3d4e5f-1283-4b94-988d-7276bdec4de2","caption":"document caption","filename":"document3.pdf"}}`},
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
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "3d4e5f6a-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"image","image":{"id":"3d4e5f6a-1283-4b94-988d-7276bdec4de2","caption":"image caption"}}`},
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
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "4e5f6a7b-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"video","video":{"id":"4e5f6a7b-1283-4b94-988d-7276bdec4de2","caption":"video caption"}}`},
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"}}}`,
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
		Label:     "Template Send with image header",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header/media", "name": "header", "variables": {"1": 0}},
				{"type": "body/text", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type": "image", "value": "image/jpeg:https://foo.bar/image.jpg"},
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send with video header",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header/media", "name": "header", "variables": {"1": 0}},
				{"type": "body/text", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type": "video", "value": "video/mp4:https://foo.bar/video.mp4"},
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"video","video":{"link":"https://foo.bar/video.mp4"}}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send with document header",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header/media", "name": "header", "variables": {"1": 0}},
				{"type": "body/text", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type": "document", "value": "application/pdf:https://foo.bar/doc.pdf"},
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"document","document":{"link":"https://foo.bar/doc.pdf","filename":"doc.pdf"}}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		// a component with no variables of its own serializes as "parameters":null
		Label:     "Template Send with static body",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header/media", "name": "header", "variables": {"1": 0}},
				{"type": "body/text", "name": "body", "variables": {}}
			],
			"variables": [
				{"type": "image", "value": "image/jpeg:https://foo.bar/image.jpg"}
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":null}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send with text header",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"components": [
				{"type": "header/text", "name": "header", "variables": {"1": 0}},
				{"type": "body/text", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type": "text", "value": "Welcome"},
				{"type": "text", "value": "Chef"},
				{"type": "text", "value": "tomorrow"}
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
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"text","text":"Welcome"}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send with button params",
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
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Ryan Lewis"},{"type":"text","text":"niño"}]},{"type":"button","sub_type":"quick_reply","index":"0","parameters":[{"type":"payload","payload":"Sip"}]},{"type":"button","sub_type":"url","index":"1","parameters":[{"type":"text","text":"id00231"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Template Send ignores preview attachments and quick replies",
		MsgText:         "This is a test image template message to check if Rapidpro",
		MsgURN:          "whatsapp:250788123123",
		MsgLocale:       "eng",
		MsgAttachments:  []string{"image:https://foo.bar/image.jpg"},
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "Test"}, {Type: "text", Text: "Another test"}},
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "test_image_rapidpro"},
			"components": [
				{"name": "header", "type": "header/media", "variables": {"1": 0}}
			],
			"variables": [
				{"type": "image", "value": "image:https://foo.bar/image.jpg"}
			],
			"language": "en"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/v1/messages",
			Body: `{"to":"250788123123","type":"template","template":{"namespace":"waba_namespace","name":"test_image_rapidpro","language":{"policy":"deterministic","code":"en"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]}]}}`,
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
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image2.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"image","image":{"link":"https://foo.bar/image2.jpg"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "ROW1"}, {Type: "text", Text: "ROW2"}, {Type: "text", Text: "ROW3"}, {Type: "text", Text: "ROW4"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image3.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/image3.jpg": {
				httpx.NewMockResponse(200, nil, []byte(`data`)),
			},
			"*/v1/media": {
				httpx.NewMockResponse(201, nil, []byte(`{ "media" : [{"id": "6a7b8c9d-1283-4b94-988d-7276bdec4de2"}] }`)),
			},
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{},
			{Body: `{"to":"250788123123","type":"image","image":{"id":"6a7b8c9d-1283-4b94-988d-7276bdec4de2"}}`},
			{Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:           "Interactive with location request",
		MsgText:         "Interactive send location",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "location"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"location_request_message","body":{"text":"Interactive send location"},"action":{"name":"send_location"}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive with form",
		MsgText:         "Interactive form msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "form", Text: "Book now", Extra: "123456"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"flow","body":{"text":"Interactive form msg"},"action":{"name":"flow","parameters":{"flow_message_version":"3","flow_id":"123456","flow_cta":"Book now"}}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive with form, with extra quick replies ignored and default CTA",
		MsgText:         "Interactive form msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "form", Extra: "123456"}, {Type: "text", Text: "Yes"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"flow","body":{"text":"Interactive form msg"},"action":{"name":"flow","parameters":{"flow_message_version":"3","flow_id":"123456","flow_cta":"Open Form"}}}}`,
		}},
		ExpectedExtIDs:    []string{"157b5e14568e8"},
		ExpectedLogErrors: []*svclogs.Error{{Message: "quick reply of type text can't be combined with a form quick reply and won't be sent"}},
	},
	{
		Label:           "Interactive with form missing form ID, sent as text",
		MsgText:         "Interactive form msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "form", Text: "Book now"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"text","text":{"body":"Interactive form msg"}}`,
		}},
		ExpectedExtIDs:    []string{"157b5e14568e8"},
		ExpectedLogErrors: []*svclogs.Error{{Message: "quick reply of type form is missing its extra value and can't be sent"}},
	},
	{
		Label:           "Interactive with URL button",
		MsgText:         "Interactive URL msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"cta_url","body":{"text":"Interactive URL msg"},"action":{"name":"cta_url","parameters":{"display_text":"Visit","url":"https://example.com"}}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive with URL button, with extra quick replies ignored and default display text",
		MsgText:         "Interactive URL msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Extra: "https://example.com"}, {Type: "text", Text: "Yes"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"cta_url","body":{"text":"Interactive URL msg"},"action":{"name":"cta_url","parameters":{"display_text":"Open Link","url":"https://example.com"}}}}`,
		}},
		ExpectedExtIDs:    []string{"157b5e14568e8"},
		ExpectedLogErrors: []*svclogs.Error{{Message: "quick reply of type text can't be combined with a url quick reply and won't be sent"}},
	},
	{
		Label:           "Interactive with URL button, with attachment used as header",
		MsgText:         "Interactive URL msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit", Extra: "https://example.com"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image2.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"interactive","interactive":{"type":"cta_url","header":{"type":"image","image":{"link":"https://foo.bar/image2.jpg"}},"body":{"text":"Interactive URL msg"},"action":{"name":"cta_url","parameters":{"display_text":"Visit","url":"https://example.com"}}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive with URL button missing URL, sent as text",
		MsgText:         "Interactive URL msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"to":"250788123123","type":"text","text":{"body":"Interactive URL msg"}}`,
		}},
		ExpectedExtIDs:    []string{"157b5e14568e8"},
		ExpectedLogErrors: []*svclogs.Error{{Message: "quick reply of type url is missing its extra value and can't be sent"}},
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
		ExpectedError: channels.ErrConnectionThrottled,
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
		ExpectedError: channels.ErrConnectionThrottled,
	},
	{
		Label:   "Error Turn HTTP 429 Rate Limit Bucket",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(429, map[string]string{
					"Content-Type":          "application/json; charset=utf-8",
					"Retry-After":           "1",
					"X-Ratelimit-Bucket":    "text",
					"X-Ratelimit-Limit":     "20",
					"X-Ratelimit-Remaining": "0",
					"X-Ratelimit-Reset":     "1786331727.285",
				}, []byte(`{"errors":[{"code":429,"title":"Rate limit hit for bucket text","details":"You are being rate limited by Turn. please read the documentation at https://whatsapp.turn.io/docs/"}]}`)),
			},
		},
		ExpectedError: channels.ErrConnectionThrottled,
	},
	{
		Label:   "Error Retryable",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(400, nil, []byte(`{ "error": {"message": "Media upload error","code": 131053 }}`)),
			},
		},
		ExpectedError: channels.ErrRetryableWithReason("131053", "Media upload error"),
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
		ExpectedError: channels.ErrFailedWithReason("368", "(#368) Temporarily blocked for policies violations"),
	},
	{
		Label:   "Error Message",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/v1/messages": {
				httpx.NewMockResponse(403, nil, []byte(`{ "error": {"message": "Other error with message","code": 0 }}`)),
			},
		},
		ExpectedError: channels.ErrFailedWithReason("0", "Other error with message"),
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
		ExpectedError: channels.ErrConnectionFailed,
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
		},
		ExpectedRequests: []ExpectedRequest{
			{},
			{Body: "media bytes"},
		},
		ExpectedError: channels.ErrRetryableWithReason("media_upload_failed", "unable to upload media to WhatsApp"),
	},
	{
		// the failed upload above is cached, so this one errors without re-attempting the download
		Label:          "Previous Media Upload Error",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses:  map[string][]*httpx.MockResponse{},
		ExpectedError:  channels.ErrRetryableWithReason("media_upload_failed", "unable to upload media to WhatsApp"),
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
	failedMediaCache.Clear()
	RunOutgoingTestCases(t, channel, newHandler(), mediaCacheSendTestCases, []string{"a123"}, nil)
	failedMediaCache.Clear()
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
