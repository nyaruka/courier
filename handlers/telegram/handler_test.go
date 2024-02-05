package telegram

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TG", "2020", "US", map[string]any{"auth_token": "a123"}),
}

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
			{Type: courier.EventTypeNewConversation, URN: "telegram:3527065#nicpottier", Time: time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)},
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
		ExpectedMsgText:      Sp("-2.890287,-79.004333"),
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
		ExpectedErrors:       []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
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
		ExpectedErrors:       []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
	},
	{
		Label:                "Receive error File response",
		URL:                  "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/",
		Data:                 errorFile,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to resolve file",
		ExpectedErrors:       []*courier.ChannelError{courier.ErrorExternal("500", "error loading file")},
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

	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	apiURL = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:              "Plain Send",
		MsgText:            "Simple Message",
		MsgURN:             "telegram:12345",
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"text":         "Simple Message",
			"chat_id":      "12345",
			"reply_markup": `{"remove_keyboard":true}`,
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Quick Reply",
		MsgText:            "Are you happy?",
		MsgURN:             "telegram:12345",
		MsgQuickReplies:    []string{"Yes", "No"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"text":         "Are you happy?",
			"chat_id":      "12345",
			"reply_markup": `{"keyboard":[[{"text":"Yes"},{"text":"No"}]],"resize_keyboard":true,"one_time_keyboard":true}`,
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Quick Reply with multiple attachments",
		MsgText:            "Are you happy?",
		MsgURN:             "telegram:12345",
		MsgQuickReplies:    []string{"Yes", "No"},
		MsgAttachments:     []string{"application/pdf:https://foo.bar/doc1.pdf", "application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"chat_id":      "12345",
			"document":     "https://foo.bar/document.pdf",
			"reply_markup": `{"keyboard":[[{"text":"Yes"},{"text":"No"}]],"resize_keyboard":true,"one_time_keyboard":true}`,
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},

	{
		Label:              "Unicode Send",
		MsgText:            "☺",
		MsgURN:             "telegram:12345",
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"text": "☺", "chat_id": "12345"},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "telegram:12345",
		MockResponseBody:   `{ "ok": false, "error_code":400, "description":"Bot domain invalid." }`,
		MockResponseStatus: 403,
		ExpectedPostParams: map[string]string{"text": `Error`, "chat_id": "12345"},
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("400", "Bot domain invalid.")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Stopped Contact Code",
		MsgText:            "Stopped Contact",
		MsgURN:             "telegram:12345",
		MockResponseBody:   `{ "ok": false, "error_code":403, "description":"Forbidden: bot was blocked by the user"}`,
		MockResponseStatus: 403,
		ExpectedPostParams: map[string]string{"text": `Stopped Contact`, "chat_id": "12345"},
		ExpectedMsgStatus:  "F",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("403", "Forbidden: bot was blocked by the user")},
		ExpectedStopEvent:  true,
		SendPrep:           setSendURL,
	},
	{
		Label:              "Should not stop other error",
		MsgText:            "Simple Message",
		MsgURN:             "telegram:12345",
		MockResponseBody:   `{ "ok": true }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{
			"text":         "Simple Message",
			"chat_id":      "12345",
			"reply_markup": `{"remove_keyboard":true}`,
		},
		ExpectedMsgStatus: "E",
		SendPrep:          setSendURL,
	},
	{
		Label:              "Send Photo",
		MsgText:            "My pic!",
		MsgURN:             "telegram:12345",
		MsgAttachments:     []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"caption": "My pic!", "chat_id": "12345", "photo": "https://foo.bar/image.jpg"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Video",
		MsgText:            "My vid!",
		MsgURN:             "telegram:12345",
		MsgAttachments:     []string{"video/mpeg:https://foo.bar/video.mpeg"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"caption": "My vid!", "chat_id": "12345", "video": "https://foo.bar/video.mpeg"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Audio",
		MsgText:            "My audio!",
		MsgURN:             "telegram:12345",
		MsgAttachments:     []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"caption": "My audio!", "chat_id": "12345", "audio": "https://foo.bar/audio.mp3"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Send Document",
		MsgText:            "My document!",
		MsgURN:             "telegram:12345",
		MsgAttachments:     []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:   `{ "ok": true, "result": { "message_id": 133 } }`,
		MockResponseStatus: 200,
		ExpectedPostParams: map[string]string{"caption": "My document!", "chat_id": "12345", "document": "https://foo.bar/document.pdf"},
		ExpectedMsgStatus:  "W",
		SendPrep:           setSendURL,
	},
	{
		Label:             "Unknown attachment type",
		MsgText:           "My pic!",
		MsgURN:            "telegram:12345",
		MsgAttachments:    []string{"unknown/foo:https://foo.bar/unknown.foo"},
		ExpectedMsgStatus: "E",
		ExpectedErrors:    []*courier.ChannelError{courier.ErrorMediaUnsupported("unknown/foo")},
		SendPrep:          setSendURL,
	},
}

func TestOutgoing(t *testing.T) {
	var defaultChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		map[string]any{courier.ConfigAuthToken: "auth_token"})

	RunOutgoingTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, []string{"auth_token"}, nil)
}
