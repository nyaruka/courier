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
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TG", "2020", "US", nil),
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

var testCases = []ChannelTestCase{
	{Label: "Receive Valid Message", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: helloMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Hello World"), URN: Sp("telegram:3527065"), External: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},

	{Label: "Receive No Params", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: emptyMsg, Status: 200, Response: "Ignoring"},

	{Label: "Receive Invalid JSON", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: "foo", Status: 400, Response: "unable to parse"},

	{Label: "Receive Sticker", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: stickerMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), MediaURL: Sp("/file/bot/sticker.jpg"), URN: Sp("telegram:3527065"), External: Sp("44"), Date: Tp(time.Date(2016, 1, 30, 2, 07, 48, 0, time.UTC))},

	{Label: "Receive Photo", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: photoMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Photo Caption"), MediaURL: Sp("/file/bot/photo.jpg"), URN: Sp("telegram:3527065"), External: Sp("85"), Date: Tp(time.Date(2017, 5, 3, 20, 28, 38, 0, time.UTC))},

	{Label: "Receive Video", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: videoMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), MediaURL: Sp("/file/bot/video.jpg"), URN: Sp("telegram:3527065"), External: Sp("86"), Date: Tp(time.Date(2017, 5, 3, 20, 29, 24, 0, time.UTC))},

	{Label: "Receive Voice", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: voiceMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), MediaURL: Sp("/file/bot/voice.mp4"), URN: Sp("telegram:3527065"), External: Sp("91"), Date: Tp(time.Date(2017, 5, 3, 20, 50, 46, 0, time.UTC))},

	{Label: "Receive Document", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: documentMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), MediaURL: Sp("/file/bot/document.xls"), URN: Sp("telegram:3527065"), External: Sp("92"), Date: Tp(time.Date(2017, 5, 3, 20, 58, 20, 0, time.UTC))},

	{Label: "Receive Location", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: locationMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("-2.890287,-79.004333"), MediaURL: Sp("geo:-2.890287,-79.004333"), URN: Sp("telegram:3527065"), External: Sp("94"), Date: Tp(time.Date(2017, 5, 3, 21, 00, 44, 0, time.UTC))},

	{Label: "Receive Venue", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: venueMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Cuenca, Provincia del Azuay"), MediaURL: Sp("geo:-2.898944,-79.006835"), URN: Sp("telegram:3527065"), External: Sp("95"), Date: Tp(time.Date(2017, 5, 3, 21, 05, 20, 0, time.UTC))},

	{Label: "Receive Contact", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: contactMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Adolf Taxi (0788531373)"), URN: Sp("telegram:3527065"), External: Sp("96"), Date: Tp(time.Date(2017, 5, 3, 21, 9, 15, 0, time.UTC))},

	{Label: "Receive Empty", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: emptyMsg, Status: 200, Response: "Ignoring"},

	{Label: "Receive Invalid FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: invalidFileID, Status: 400, Response: "error retrieving media"},

	{Label: "Receive NoOk FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: noOkFile, Status: 400, Response: "no 'ok' in response"},

	{Label: "Receive NotOk FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: notOkFile, Status: 400, Response: "not present"},

	{Label: "Receive No FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: noFile, Status: 400, Response: "result.file_path"},
}

func buildMockTelegramService(testCases []ChannelTestCase) *httptest.Server {
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

	telegramAPIURL = server.URL

	// update our tests media urls
	for c := range testCases {
		if testCases[c].MediaURL != nil && !strings.HasPrefix(*testCases[c].MediaURL, "geo") {
			testCases[c].MediaURL = Sp(fmt.Sprintf("%s%s", telegramAPIURL, *testCases[c].MediaURL))
		}
	}

	return server
}

func TestHandler(t *testing.T) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	RunChannelTestCases(t, testChannels, NewHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	RunChannelBenchmarks(b, testChannels, NewHandler(), testCases)
}
