package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/svclogs"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
)

var helloMsg = `{
  "update_id": 174114370,
  "message": {
	"message_id": 41,
	"from": {
		"id": 3527065,
		"first_name": "Nic",
		"last_name": "Pottier",
		"username": "nicpottier"
	},
	"chat": {
		"id": 3527065,
		"first_name": "Nic",
		"last_name": "Pottier",
		"type": "private"
	},
	"date": 1454119029,
	"text": "Hello World"
  }
}`

var startMsg = `{
    "update_id": 174114370,
    "message": {
      "message_id": 41,
      "from": {
          "id": 3527065,
          "first_name": "Nic",
          "last_name": "Pottier",
          "username": "nicpottier"
      },
      "chat": {
          "id": 3527065,
          "first_name": "Nic",
          "last_name": "Pottier",
          "type": "private"
      },
      "date": 1454119029,
      "text": "/start"
    }
  }`

var emptyMsg = `{
 	"update_id": 174114370
}`

var stickerMsg = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"AAQDABNW--sqAAS6easb1s1rNdJYAAIC",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var invalidFileID = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"invalid",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var notOkFile = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"notok",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var noOkFile = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"nook",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var invalidJsonFile = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"invalidjson",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var errorFile = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"error",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var noFile = `
{
  "update_id":174114373,
  "message":{
    "message_id":44,
    "from":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier"
    },
    "chat":{
      "id":3527065,
      "first_name":"Nic",
      "last_name":"Pottier",
      "type":"private"
    },
    "date":1454119668,
    "sticker":{
      "width":436,
      "height":512,
      "thumb":{
        "file_id":"nofile",
        "file_size":2510,
        "width":77,
        "height":90
      },
      "file_id":"BQADAwADRQADyIsGAAHtBskMy6GoLAI",
      "file_size":38440
    }
  }
}`

var photoMsg = `
{
    "update_id": 900946525,
    "message": {
        "message_id": 85,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493843318,
        "photo": [
            {
                "file_id": "AgADAQADtKcxG4LRUUQSQVUjfJIiiF8G6C8ABHsRSbk65AmUi3cBAAEC",
                "file_size": 1140,
                "width": 51,
                "height": 90
            },
            {
                "file_id": "AgADAQADtKcxG4LRUUQSQVUjfJIiiF8G6C8ABNEDQTuwtue6jXcBAAEC",
                "file_size": 12138,
                "width": 180,
                "height": 320
            },
            {
                "file_id": "AgADAQADtKcxG4LRUUQSQVUjfJIiiF8G6C8ABF8Fy2sccmWmjHcBAAEC",
                "file_size": 57833,
                "width": 450,
                "height": 800
            },
            {
                "file_id": "AgADAQADtKcxG4LRUUQSQVUjfJIiiF8G6C8ABA9NJzFdXskaincBAAEC",
                "file_size": 104737,
                "width": 720,
                "height": 1280
            }
        ],
        "caption": "Photo Caption"
    }
}`

var videoMsg = `
{
    "update_id": 900946526,
    "message": {
        "message_id": 86,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493843364,
        "video": {
            "duration": 1,
            "width": 360,
            "height": 640,
            "mime_type": "video/mp4",
            "thumb": {
                "file_id": "AAQBABP2RvcvAATGjpC2zjwhKQ8xAAIC",
                "file_size": 1770,
                "width": 50,
                "height": 90
            },
            "file_id": "BAADAQADBgADgtFRRPFTAAHxLVw76wI",
            "file_size": 257507
        }
    }
}`

var voiceMsg = `
{
    "update_id": 900946531,
    "message": {
        "message_id": 91,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493844646,
        "voice": {
            "duration": 1,
            "mime_type": "audio/ogg",
            "file_id": "AwADAQADCQADgtFRRGn8KrC-0D_MAg",
            "file_size": 4288
        }
    }
}`

var documentMsg = `
{
    "update_id": 900946532,
    "message": {
        "message_id": 92,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845100,
        "document": {
            "file_name": "TabFig2015prel.xls",
            "mime_type": "application/vnd.ms-excel",
            "file_id": "BQADAQADCgADgtFRRPrv9GQ95f8eAg",
            "file_size": 4540928
        }
    }
}`

var locationMsg = `
{
    "update_id": 900946534,
    "message": {
        "message_id": 94,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845244,
        "location": {
            "latitude": -2.890287,
            "longitude": -79.004333
        }
    }
}`

var venueMsg = `
{
    "update_id": 900946535,
    "message": {
        "message_id": 95,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845520,
        "location": {
            "latitude": -2.898944,
            "longitude": -79.006835
        },
        "venue": {
            "location": {
                "latitude": -2.898944,
                "longitude": -79.006835
            },
            "title": "Cuenca",
            "address": "Provincia del Azuay",
            "foursquare_id": "4c21facd9a67a59340acdb87"
        }
    }
}`

var contactMsg = `
{
    "update_id": 900946536,
    "message": {
        "message_id": 96,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845755,
        "contact": {
            "phone_number": "0788531373",
            "first_name": "Adolf Taxi"
        }
    }
}`

var webAppDataMsg = `
{
    "update_id": 900946537,
    "message": {
        "message_id": 97,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845755,
        "web_app_data": {
            "data": "{\"first_name\": \"Bob\", \"age\": \"32\"}",
            "button_text": "Register"
        }
    }
}`

var webAppDataNonJSONMsg = `
{
    "update_id": 900946538,
    "message": {
        "message_id": 98,
        "from": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier"
        },
        "chat": {
            "id": 3527065,
            "first_name": "Nic",
            "last_name": "Pottier",
            "username": "Nicpottier",
            "type": "private"
        },
        "date": 1493845755,
        "web_app_data": {
            "data": "bob,32",
            "button_text": "Register"
        }
    }
}`

var testCases = []IncomingTestCase{
	{

		Label:                "Receive Valid Message",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 helloMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "41",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{

		Label:                "Receive Start Message",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 startMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedEvents: []ExpectedEvent{
			{Type: models.EventTypeNewConversation, URN: "telegram:3527065#nicpottier", Time: time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)},
		},
	},
	{
		Label:                "Receive No Params",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 emptyMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Ignoring",
	},
	{
		Label:                "Receive Invalid JSON",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 "foo",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive Sticker",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 stickerMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"/file/bota123/sticker.jpg"},
		ExpectedURN:          "telegram:3527065",
		ExpectedExternalID:   "44",
		ExpectedDate:         time.Date(2016, 1, 30, 2, 07, 48, 0, time.UTC),
	},
	{
		Label:                "Receive Photo",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 photoMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("Photo Caption"),
		ExpectedAttachments:  []string{"/file/bota123/photo.jpg"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "85",
		ExpectedDate:         time.Date(2017, 5, 3, 20, 28, 38, 0, time.UTC),
	},
	{
		Label:                "Receive Video",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 videoMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"/file/bota123/video.jpg"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "86",
		ExpectedDate:         time.Date(2017, 5, 3, 20, 29, 24, 0, time.UTC),
	},
	{
		Label:                "Receive Voice",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 voiceMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"/file/bota123/voice.mp4"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "91",
		ExpectedDate:         time.Date(2017, 5, 3, 20, 50, 46, 0, time.UTC),
	},
	{
		Label:                "Receive Document",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 documentMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"/file/bota123/document.xls"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "92",
		ExpectedDate:         time.Date(2017, 5, 3, 20, 58, 20, 0, time.UTC),
	},
	{
		Label:                "Receive Location",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 locationMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:-2.890287,-79.004333"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "94",
		ExpectedDate:         time.Date(2017, 5, 3, 21, 00, 44, 0, time.UTC),
	},
	{
		Label:                "Receive Venue",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 venueMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("Cuenca, Provincia del Azuay"),
		ExpectedAttachments:  []string{"geo:-2.898944,-79.006835"},
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "95",
		ExpectedDate:         time.Date(2017, 5, 3, 21, 05, 20, 0, time.UTC),
	},
	{
		Label:                "Receive Contact",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 contactMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("Adolf Taxi (0788531373)"),
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "96",
		ExpectedDate:         time.Date(2017, 5, 3, 21, 9, 15, 0, time.UTC),
	},
	{
		Label:                "Receive WebApp Data",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 webAppDataMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("Register"),
		ExpectedPayload:      `{"first_name": "Bob", "age": "32"}`,
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "97",
		ExpectedDate:         time.Date(2017, 5, 3, 21, 9, 15, 0, time.UTC),
	},
	{
		Label:                "Receive WebApp Data non-JSON",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 webAppDataNonJSONMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Accepted",
		ExpectedContactName:  Sp("Nic Pottier"),
		ExpectedMsgText:      Sp("bob,32"),
		ExpectedURN:          "telegram:3527065#nicpottier",
		ExpectedExternalID:   "98",
		ExpectedDate:         time.Date(2017, 5, 3, 21, 9, 15, 0, time.UTC),
		ExpectedErrors:       []*svclogs.Error{{Message: "web_app_data data is not a valid JSON object"}},
	},
	{
		Label:                "Receive Empty",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 emptyMsg,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Ignoring",
	},
	{
		Label:                "Receive Invalid FileID",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 invalidFileID,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to resolve file",
		ExpectedErrors:       []*svclogs.Error{models.ErrorResponseUnparseable("JSON")},
	},
	{
		Label:                "Receive NoOk FileID",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 noOkFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "not present",
	},
	{
		Label:                "Receive invalid JSON File response",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 invalidJsonFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to resolve file",
		ExpectedErrors:       []*svclogs.Error{models.ErrorResponseUnparseable("JSON")},
	},
	{
		Label:                "Receive error File response",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 errorFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to resolve file",
		ExpectedErrors:       []*svclogs.Error{models.ErrorExternal("500", "error loading file")},
	},
	{
		Label:                "Receive NotOk FileID",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 notOkFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "not present",
	},
	{
		Label:                "Receive No FileID",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 noFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "result.file_path",
	},
}

func buildMockTelegramService(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileID := r.FormValue("file_id")
		defer r.Body.Close()

		filePath := ""

		switch fileID {
		case "AAQDABNW--sqAAS6easb1s1rNdJYAAIC":
			filePath = "sticker.jpg"
		case "AgADAQADtKcxG4LRUUQSQVUjfJIiiF8G6C8ABF8Fy2sccmWmjHcBAAEC":
			filePath = "photo.jpg"
		case "BAADAQADBgADgtFRRPFTAAHxLVw76wI":
			filePath = "video.jpg"
		case "AwADAQADCQADgtFRRGn8KrC-0D_MAg":
			filePath = "voice.mp4"
		case "BQADAQADCgADgtFRRPrv9GQ95f8eAg":
			filePath = "document.xls"
		case "notok":
			w.Write([]byte(`{ "ok": false, "result": { "file_path": "nothing" } }`))
			return
		case "invalidjson":
			w.Write([]byte(`invalid`))
			return
		case "error":
			w.Write([]byte(`{ "error_code": 500, "description": "error loading file" }`))
			return
		case "nook":
			w.Write([]byte(`{}`))
			return
		case "nofile":
			w.Write([]byte(`{ "ok": true, "result": {} }`))
			return
		}

		if filePath == "" {
			http.Error(w, "unknown file id", 400)
		}

		w.Write([]byte(fmt.Sprintf(`{ "ok": true, "result": { "file_path": "%s" } }`, filePath)))
	}))

	apiURL = server.URL

	// update our tests media urls
	for _, tc := range testCases {
		for i := range tc.ExpectedAttachments {
			if !strings.HasPrefix(tc.ExpectedAttachments[i], "geo:") {
				tc.ExpectedAttachments[i] = fmt.Sprintf("%s%s", apiURL, tc.ExpectedAttachments[i])
			}
		}
	}

	return server
}

func TestIncoming(t *testing.T) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TG", "2020", "US", []string{urns.Telegram.Prefix}, map[string]any{"auth_token": "a123"}),
	}

	RunIncomingTestCases(t, chs, newHandler(), testCases)
}

var outgoingCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "telegram:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Simple Message"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Yes"},{"text":"No"}]],"resize_keyboard":true,"one_time_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, request location",
		MsgText:         "Where Are you?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "location", Text: "Send Location"}, {Type: "text", Text: "Ignore"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Where Are you?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Send Location","request_location":true},{"text":"Ignore"}]],"resize_keyboard":true,"one_time_keyboard":false}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, form type opens Mini App",
		MsgText:         "Please register",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "form", Text: "Register", Extra: "https://example.com/form"}, {Type: "text", Text: "Skip"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Please register"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Register","web_app":{"url":"https://example.com/form"}},{"text":"Skip"}]],"resize_keyboard":true,"one_time_keyboard":false}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, form type without a URL is dropped",
		MsgText:         "Please register",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "form", Text: "Register"}, {Type: "text", Text: "Skip"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Please register"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Skip"}]],"resize_keyboard":true,"one_time_keyboard":true}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type form is missing its extra value and can't be sent"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url types as inline keyboard link buttons",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit Us", Extra: "http://example.com"}, {Type: "url", Extra: "http://example.com/more"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"inline_keyboard":[[{"text":"Visit Us","url":"http://example.com"},{"text":"Open Link","url":"http://example.com/more"}]]}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url type mixed with reply keyboard types is dropped",
		MsgText:         "Please register",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Extra: "http://example.com"}, {Type: "form", Text: "Register", Extra: "https://example.com/form"}, {Type: "text", Text: "Skip"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Please register"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Register","web_app":{"url":"https://example.com/form"}},{"text":"Skip"}]],"resize_keyboard":true,"one_time_keyboard":false}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type url isn't supported by this channel and can't be sent"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url type with attachment",
		MsgText:         "Check this out",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit", Extra: "http://example.com"}},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendPhoto": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"caption": {"Check this out"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "photo": {"https://foo.bar/image.jpg"}, "reply_markup": {`{"inline_keyboard":[[{"text":"Visit","url":"http://example.com"}]]}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, unknown type dropped",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "video", Text: "Play"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type video isn't supported by this channel and can't be sent"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url types with invalid URLs are dropped",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit Us", Extra: "example.com/visit"}, {Type: "url", Text: "Local", Extra: "http://localhost/visit"}, {Type: "url", Text: "Spaced", Extra: "https://example.com/a page"}, {Type: "url", Text: "Read More", Extra: "https://example.com/more"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"inline_keyboard":[[{"text":"Read More","url":"https://example.com/more"}]]}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type url has an invalid URL and can't be sent: example.com/visit"},
			{Message: "quick reply of type url has an invalid URL and can't be sent: http://localhost/visit"},
			{Message: "quick reply of type url has an invalid URL and can't be sent: https://example.com/a page"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url type without a URL is dropped",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit Us"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type url is missing its extra value and can't be sent"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url type mixed with location is dropped",
		MsgText:         "Where are you?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "location"}, {Type: "url", Extra: "http://example.com"}},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Where are you?"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Send Location","request_location":true}]],"resize_keyboard":true,"one_time_keyboard":false}`}}},
		},
		ExpectedLogErrors: []*svclogs.Error{
			{Message: "quick reply of type url isn't supported by this channel and can't be sent"},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply, url type with multiple attachments",
		MsgText:         "Check this out",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "url", Text: "Visit", Extra: "http://example.com"}},
		MsgAttachments:  []string{"application/pdf:https://foo.bar/doc1.pdf", "application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
			"*/botauth_token/sendDocument": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Check this out"}, "chat_id": {"12345"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
			{Form: url.Values{"caption": []string{""}, "chat_id": {"12345"}, "document": {"https://foo.bar/doc1.pdf"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
			{Form: url.Values{"caption": []string{""}, "chat_id": {"12345"}, "document": {"https://foo.bar/document.pdf"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"inline_keyboard":[[{"text":"Visit","url":"http://example.com"}]]}`}}},
		},
		ExpectedExtIDs: []string{"133", "133", "133"},
	},
	{
		Label:           "Quick Reply with multiple attachments",
		MsgText:         "Are you happy?",
		MsgURN:          "telegram:12345",
		MsgQuickReplies: []models.QuickReply{{Type: "text", Text: "Yes"}, {Type: "text", Text: "No"}},
		MsgAttachments:  []string{"application/pdf:https://foo.bar/doc1.pdf", "application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
			"*/botauth_token/sendDocument": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Are you happy?"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
			{Form: url.Values{"caption": []string{""}, "chat_id": {"12345"}, "document": {"https://foo.bar/doc1.pdf"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
			{Form: url.Values{"caption": []string{""}, "chat_id": {"12345"}, "document": {"https://foo.bar/document.pdf"}, "parse_mode": {"Markdown"}, "reply_markup": {`{"keyboard":[[{"text":"Yes"},{"text":"No"}]],"resize_keyboard":true,"one_time_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133", "133", "133"},
	},
	{
		Label:   "Error",
		MsgText: "Error",
		MsgURN:  "telegram:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(403, nil, []byte(`{ "ok": false, "error_code":400, "description":"Bot domain invalid." }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Error"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedError: channels.ErrFailedWithReason("400", "Bot domain invalid."),
	},
	{
		Label:   "Throttled",
		MsgText: "Error",
		MsgURN:  "telegram:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(429, nil, []byte(`{ "ok": false, "error_code":429, "description":"Too Many Requests: retry after 30", "parameters": {"retry_after": 30} }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Error"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedError: channels.ErrConnectionThrottled,
	},
	{
		Label:   "Stopped Contact Code",
		MsgText: "Stopped Contact",
		MsgURN:  "telegram:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(403, nil, []byte(`{ "ok": false, "error_code":403, "description":"Forbidden: bot was blocked by the user"}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Stopped Contact"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedError: channels.ErrContactStopped,
	},
	{
		Label:          "Send Photo",
		MsgText:        "My pic!",
		MsgURN:         "telegram:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendPhoto": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"caption": {"My pic!"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "photo": {"https://foo.bar/image.jpg"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:          "Send Video",
		MsgText:        "My vid!",
		MsgURN:         "telegram:12345",
		MsgAttachments: []string{"video/mpeg:https://foo.bar/video.mpeg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendVideo": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"caption": {"My vid!"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "video": {"https://foo.bar/video.mpeg"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:          "Send Audio",
		MsgText:        "My audio!",
		MsgURN:         "telegram:12345",
		MsgAttachments: []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendAudio": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"caption": {"My audio!"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "audio": {"https://foo.bar/audio.mp3"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:          "Send Document",
		MsgText:        "My document!",
		MsgURN:         "telegram:12345",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendDocument": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 133 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"caption": {"My document!"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "document": {"https://foo.bar/document.pdf"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:   "Response Unexpected",
		MsgText: "Simple Message",
		MsgURN:  "telegram:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/botauth_token/sendMessage": {
				httpx.NewMockResponse(200, nil, []byte(`{ "ok": true, "result": { "message_id": 0 } }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Form: url.Values{"text": {"Simple Message"}, "chat_id": {"12345"}, "parse_mode": []string{"Markdown"}, "reply_markup": {`{"remove_keyboard":true}`}}},
		},
		ExpectedError: channels.ErrResponseContent,
	},
	{
		Label:             "Unknown attachment type",
		MsgText:           "My foo!",
		MsgURN:            "telegram:12345",
		MsgAttachments:    []string{"unknown/foo:https://foo.bar/unknown.foo"},
		ExpectedLogErrors: []*svclogs.Error{models.ErrorMediaUnsupported("unknown/foo")},
	},
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		[]string{urns.Telegram.Prefix},
		map[string]any{models.ConfigAuthToken: "auth_token"},
	)

	RunOutgoingTestCases(t, ch, newHandler(), outgoingCases, []string{"auth_token"}, nil)
}

func TestSendEvent(t *testing.T) {
	// other tests repoint apiURL at mock servers, so pin it for this test
	defer func(u string) { apiURL = u }(apiURL)
	apiURL = "https://api.telegram.org"

	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	h := newHandler().(*handler)
	s.MountHandler(h)

	s.Runtime().HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://api.telegram.org/botauth_token/sendChatAction": {
			httpx.NewMockResponse(200, nil, []byte(`{"ok": true, "result": true}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"ok": false, "error_code": 400, "description": "Bad Request"}`)),
			httpx.MockConnectionError,
		},
	})

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		[]string{urns.Telegram.Prefix},
		map[string]any{models.ConfigAuthToken: "auth_token"},
	)

	typing := events.NewTypingStarted(events.DirectionOutgoing, assets.NewChannelReference("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "Telegram"), "telegram:12345", "")

	clog := models.NewChannelLogForEventSend(ch, nil)
	err := h.SendEvent(context.Background(), ch, typing, clog)
	assert.NoError(t, err)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://api.telegram.org/botauth_token/sendChatAction", clog.HttpLogs[0].URL)
	assert.Contains(t, clog.HttpLogs[0].Request, "chat_id=12345")
	assert.Contains(t, clog.HttpLogs[0].Request, "action=typing")

	// typing indicators display for ~5 seconds so should be resent more often than that to sustain
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 4 * time.Second}, h.SendableEvents(ch))

	// non-ok response is a response error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrResponseStatus, err)

	// as is a connection error
	err = h.SendEvent(context.Background(), ch, typing, clog)
	assert.Equal(t, channels.ErrConnectionFailed, err)

	// channel without an auth token can't send
	noAuth := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US", []string{urns.Telegram.Prefix}, map[string]any{})
	err = h.SendEvent(context.Background(), noAuth, typing, clog)
	assert.Equal(t, channels.ErrChannelConfig, err)

	// an event type the handler doesn't declare support for can't be sent
	err = h.SendEvent(context.Background(), ch, events.NewTypingStopped(events.DirectionOutgoing, nil, "telegram:12345", ""), clog)
	assert.ErrorContains(t, err, "unsupported event type: typing_stopped")
}

func TestIsValidButtonURL(t *testing.T) {
	valid := []string{
		"http://example.com",
		"https://example.com/path?x=1#y",
		"https://example.com:8080/path",
		"http://127.0.0.1/dev",
		"http://[::1]/dev",
		"https://exämple.com/ü",
		"tg://user?id=123456",
		"tg://resolve?domain=example",
	}
	for _, s := range valid {
		assert.True(t, isValidButtonURL(s), "expected %s to be valid", s)
	}

	invalid := []string{
		"",
		"example.com",
		"www.example.com/path",
		"mailto:bob@example.com",
		"ftp://example.com/file",
		"http://",
		"http://localhost/dev",
		"https://example.com/a page",
		"https://example .com",
	}
	for _, s := range invalid {
		assert.False(t, isValidButtonURL(s), "expected %s to be invalid", s)
	}
}
