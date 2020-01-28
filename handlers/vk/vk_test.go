package vk

import (
	"context"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/vk/" + channelUUID + "/receive"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel(
		channelUUID,
		"VK",
		"123456789",
		"",
		map[string]interface{}{
			courier.ConfigAuthToken:        "token123xyz",
			courier.ConfigSecret:           "abc123xyz",
			configServerVerificationString: "a1b2c3",
		}),
}

const eventWithSecret = `{
	"type": "some_event",
	"object": {},
	"secret": "abc123xyz"
}`
const eventWithoutSecret = `{
	"type": "some_event",
	"object": {}
}`

func TestCheckSecret(t *testing.T) {
	testCases := []handlers.ChannelHandleTestCase{
		{Label: "Validate secret", URL: receiveURL, Data: eventWithSecret, Status: 200, Response: "no message or server verification event"},
		{Label: "Invalidate secret", URL: receiveURL, Data: eventWithoutSecret, Status: 400, Response: "wrong secret key"},
	}
	handlers.RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

const eventServerVerification = `{
	"type": "confirmation",
	"secret": "abc123xyz"
}`

func TestServerVerification(t *testing.T) {
	testCases := []handlers.ChannelHandleTestCase{
		{Label: "Verify server", URL: receiveURL, Data: eventServerVerification, Status: 200, Response: "a1b2c3"},
	}
	handlers.RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

const msgHelloWorld = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "Hello World",
			"attachments": []
		}
	},
	"secret": "abc123xyz"
}`

const msgPhotoAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [{
				"type":"photo",
				"photo": {
					"sizes": [
						{"type": "s", "url": "https://mediahost.com/s-photo.jpg", "width": 60, "height": 75},
						{"type": "m", "url": "https://mediahost.com/m-photo.jpg", "width": 104, "height": 130},
						{"type": "x", "url": "https://mediahost.com/x-photo.jpg", "width": 483, "height": 604},
						{"type": "y", "url": "https://mediahost.com/y-photo.jpg", "width": 646, "height": 807}
					]
				}
			}]
		}
	},
	"secret": "abc123xyz"
}`

const msgGraffitiAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [{
				"type": "graffiti",
				"graffiti": { "url": "https://mediahost.com/graffiti.png" }
			}]
		}
	},
	"secret": "abc123xyz"
}`

const msgStickerAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [{
				"type": "sticker",
				"sticker": {
					"images": [
						{ "url": "https://mediahost.com/64x64_sticker.png", "width": 64, "height": 64 },
						{ "url": "https://mediahost.com/128x128_sticker.png", "width": 128, "height": 128 },
						{ "url": "https://mediahost.com/256x256_sticker.png", "width": 256, "height": 256 }
					]
				}
			}]
		}
	},
	"secret": "abc123xyz"
}`

const msgAudioAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [{
				"type": "audio_message",
				"audio_message": { "link_mp3": "https://mediahost.com/audio.mp3" }
			}]
		}
	},
	"secret": "abc123xyz"
}`

const msgDocAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [{
				"type": "doc",
				"doc": { "url": "https://mediahost.com/doc.pdf" }
			}]
		}
	},
	"secret": "abc123xyz"
}`

const msgGeoAttachment = `{
	"type": "message_new",
	"object": {
		"message": {
			"id": 1,
			"date": 1580125800,
			"from_id": 123456,
			"text": "",
			"attachments": [],
			"geo": { "coordinates": { "latitude": -9.652278, "longitude": -35.701095} }
		}
	},
	"secret": "abc123xyz"
}`

func TestNewMessage(t *testing.T) {
	testCases := []handlers.ChannelHandleTestCase{
		{Label: "Receive Message", URL: receiveURL, Data: msgHelloWorld, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)),
		},
		{Label: "Receive First Photo Attachment", URL: receiveURL, Data: msgPhotoAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://mediahost.com/x-photo.jpg"},
		},
		{Label: "Receive First Graffiti Attachment", URL: receiveURL, Data: msgGraffitiAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://mediahost.com/graffiti.png"},
		},
		{Label: "Receive First Sticker Attachment", URL: receiveURL, Data: msgStickerAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://mediahost.com/128x128_sticker.png"},
		},
		{Label: "Receive First Audio Attachment", URL: receiveURL, Data: msgAudioAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://mediahost.com/audio.mp3"},
		},
		{Label: "Receive First Audio Attachment", URL: receiveURL, Data: msgDocAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://mediahost.com/doc.pdf"},
		},
		{Label: "Receive Geolocation Attachment", URL: receiveURL, Data: msgGeoAttachment, Status: 200, Response: "ok", URN: handlers.Sp("vk:123456"),
			ExternalID: handlers.Sp("1"), Date: handlers.Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"geo:-9.652278,-35.701095"},
		},
	}
	handlers.RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func buildMockVKService(testCases []handlers.ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, actionGetUser) {
			userId := r.URL.Query()["user_ids"][0]

			if userId == "123456789" {
				_, _ = w.Write([]byte(`{"response": [{"id": 123456789, "first_name": "John", "last_name": "Doe"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"response": []}`))
		}
	}))
	apiBaseURL = server.URL
	return server
}

func TestDescribe(t *testing.T) {
	server := buildMockVKService([]handlers.ChannelHandleTestCase{})
	defer server.Close()

	handler := newHandler().(courier.URNDescriber)
	urn, _ := urns.NewURNFromParts(urns.VKScheme, "123456789", "", "")
	data := map[string]string{ "name": "John Doe" }

	describe, err := handler.DescribeURN(context.Background(), testChannels[0], urn)
	assert.Nil(t, err)
	assert.Equal(t, data, describe)
}
