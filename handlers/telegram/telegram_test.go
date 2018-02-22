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
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TG", "2020", "US", map[string]interface{}{"auth_token": "a123"}),
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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: helloMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Hello World"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("41"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},

	{Label: "Receive Start Message", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: startMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), ChannelEvent: Sp(string(courier.NewConversation)), URN: Sp("telegram:3527065#nicpottier"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC))},

	{Label: "Receive No Params", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: emptyMsg, Status: 200, Response: "Ignoring"},

	{Label: "Receive Invalid JSON", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: "foo", Status: 400, Response: "unable to parse"},

	{Label: "Receive Sticker", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: stickerMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), Attachment: Sp("/file/bota123/sticker.jpg"), URN: Sp("telegram:3527065"), ExternalID: Sp("44"), Date: Tp(time.Date(2016, 1, 30, 2, 07, 48, 0, time.UTC))},

	{Label: "Receive Photo", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: photoMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Photo Caption"), Attachment: Sp("/file/bota123/photo.jpg"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("85"), Date: Tp(time.Date(2017, 5, 3, 20, 28, 38, 0, time.UTC))},

	{Label: "Receive Video", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: videoMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), Attachment: Sp("/file/bota123/video.jpg"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("86"), Date: Tp(time.Date(2017, 5, 3, 20, 29, 24, 0, time.UTC))},

	{Label: "Receive Voice", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: voiceMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), Attachment: Sp("/file/bota123/voice.mp4"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("91"), Date: Tp(time.Date(2017, 5, 3, 20, 50, 46, 0, time.UTC))},

	{Label: "Receive Document", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: documentMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp(""), Attachment: Sp("/file/bota123/document.xls"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("92"), Date: Tp(time.Date(2017, 5, 3, 20, 58, 20, 0, time.UTC))},

	{Label: "Receive Location", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: locationMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("-2.890287,-79.004333"), Attachment: Sp("geo:-2.890287,-79.004333"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("94"), Date: Tp(time.Date(2017, 5, 3, 21, 00, 44, 0, time.UTC))},

	{Label: "Receive Venue", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: venueMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Cuenca, Provincia del Azuay"), Attachment: Sp("geo:-2.898944,-79.006835"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("95"), Date: Tp(time.Date(2017, 5, 3, 21, 05, 20, 0, time.UTC))},

	{Label: "Receive Contact", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: contactMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nic Pottier"), Text: Sp("Adolf Taxi (0788531373)"), URN: Sp("telegram:3527065#nicpottier"), ExternalID: Sp("96"), Date: Tp(time.Date(2017, 5, 3, 21, 9, 15, 0, time.UTC))},

	{Label: "Receive Empty", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: emptyMsg, Status: 200, Response: "Ignoring"},

	{Label: "Receive Invalid FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: invalidFileID, Status: 400, Response: "error retrieving media"},

	{Label: "Receive NoOk FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: noOkFile, Status: 400, Response: "no 'ok' in response"},

	{Label: "Receive NotOk FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: notOkFile, Status: 400, Response: "not present"},

	{Label: "Receive No FileID", URL: "/c/tg/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive/", Data: noFile, Status: 400, Response: "result.file_path"},
}

func buildMockTelegramService(testCases []ChannelHandleTestCase) *httptest.Server {
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

	apiURL = server.URL

	// update our tests media urls
	for c := range testCases {
		if testCases[c].Attachment != nil && !strings.HasPrefix(*testCases[c].Attachment, "geo") {
			testCases[c].Attachment = Sp(fmt.Sprintf("%s%s", apiURL, *testCases[c].Attachment))
		}
	}

	return server
}

func TestHandler(t *testing.T) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	telegramService := buildMockTelegramService(testCases)
	defer telegramService.Close()

	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "telegram:12345",
		Status: "W", ExternalID: "133",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{
			"text":         "Simple Message",
			"chat_id":      "12345",
			"reply_markup": `{"remove_keyboard":true}`,
		},
		SendPrep: setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "telegram:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "133",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{
			"text":         "Are you happy?",
			"chat_id":      "12345",
			"reply_markup": `{"resize_keyboard":true,"one_time_keyboard":true,"keyboard":[[{"text":"Yes"},{"text":"No"}]]}`,
		},
		SendPrep: setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "telegram:12345",
		Status: "W", ExternalID: "133",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{"text": "☺", "chat_id": "12345"},
		SendPrep:   setSendURL},
	{Label: "Error",
		Text: "Error", URN: "telegram:12345",
		Status:       "E",
		ResponseBody: `{ "ok": false }`, ResponseStatus: 403,
		PostParams: map[string]string{"text": `Error`, "chat_id": "12345"},
		SendPrep:   setSendURL},
	{Label: "Send Photo",
		Text: "My pic!", URN: "telegram:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status:       "W",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{"caption": "My pic!", "chat_id": "12345", "photo": "https://foo.bar/image.jpg"},
		SendPrep:   setSendURL},
	{Label: "Send Video",
		Text: "My vid!", URN: "telegram:12345", Attachments: []string{"video/mpeg:https://foo.bar/video.mpeg"},
		Status:       "W",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{"caption": "My vid!", "chat_id": "12345", "video": "https://foo.bar/video.mpeg"},
		SendPrep:   setSendURL},
	{Label: "Send Audio",
		Text: "My audio!", URN: "telegram:12345", Attachments: []string{"audio/mp3:https://foo.bar/audio.mp3"},
		Status:       "W",
		ResponseBody: `{ "ok": true, "result": { "message_id": 133 } }`, ResponseStatus: 200,
		PostParams: map[string]string{"caption": "My audio!", "chat_id": "12345", "audio": "https://foo.bar/audio.mp3"},
		SendPrep:   setSendURL},
	{Label: "Unknown Attachment",
		Text: "My pic!", URN: "telegram:12345", Attachments: []string{"unknown/foo:https://foo.bar/unknown.foo"},
		Status:   "E",
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TG", "2020", "US",
		map[string]interface{}{courier.ConfigAuthToken: "auth_token"})

	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
