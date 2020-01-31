package vk

import (
	"context"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
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
const msgEmpty = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": []
    }
  },
  "secret": "abc123xyz"
}`
const msgFirstPhotoAttachment = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": [
        {
          "type":"photo",
          "photo": {
            "sizes": [
              {"type": "s", "url": "https://foo.bar/s-photo.jpg", "width": 60, "height": 75},
              {"type": "m", "url": "https://foo.bar/m-photo.jpg", "width": 104, "height": 130},
              {"type": "x", "url": "https://foo.bar/x-photo.jpg", "width": 483, "height": 604},
              {"type": "y", "url": "https://foo.bar/y-photo.jpg", "width": 646, "height": 807}
            ]
          }
        },
        {
          "type": "graffiti",
          "graffiti": { "url": "https://foo.bar/graffiti.png" }
        }
      ]
    }
  },
  "secret": "abc123xyz"
}`
const msgFirstGraffitiAttachment = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": [
        {
          "type": "graffiti",
          "graffiti": { "url": "https://foo.bar/graffiti.png" }
        },
        {
          "type": "audio_message",
          "audio_message": { "link_mp3": "https://foo.bar/audio.mp3" }
        }
      ]
    }
  },
  "secret": "abc123xyz"
}`
const msgFirstStickerAttachment = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": [
        {
          "type": "sticker",
          "sticker": {
            "images": [
              { "url": "https://foo.bar/64x64_sticker.png", "width": 64, "height": 64 },
              { "url": "https://foo.bar/128x128_sticker.png", "width": 128, "height": 128 },
              { "url": "https://foo.bar/256x256_sticker.png", "width": 256, "height": 256 }
            ]
          }
        },
        {
          "type": "graffiti",
          "graffiti": { "url": "https://foo.bar/graffiti.png" }
        }
      ]
    }
  },
  "secret": "abc123xyz"
}`
const msgFirstAudioAttachment = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": [
        {
          "type": "audio_message",
          "audio_message": { "link_mp3": "https://foo.bar/audio.mp3" }
        },
        {
          "type": "graffiti",
          "graffiti": { "url": "https://foo.bar/graffiti.png" }
        }
      ]
    }
  },
  "secret": "abc123xyz"
}`
const msgFirstDocAttachment = `{
  "type": "message_new",
  "object": {
    "message": {
      "id": 1,
      "date": 1580125800,
      "from_id": 123456,
      "text": "",
      "attachments": [
        {
          "type": "doc",
          "doc": { "url": "https://foo.bar/doc.pdf" }
        },
        {
          "type": "audio_message",
          "audio_message": { "link_mp3": "https://foo.bar/audio.mp3" }
        }
      ]
	}
  },
  "secret": "abc123xyz"
}`
const msgGeolocationOnly = `{
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
const eventWithSecret = `{
  "type": "some_event",
  "object": {},
  "secret": "abc123xyz"
}`
const eventWithoutSecret = `{
  "type": "some_event",
  "object": {}
}`
const eventServerVerification = `{
  "type": "confirmation",
  "secret": "abc123xyz"
}`

var testCases = []ChannelHandleTestCase{
	{
		Label:      "Receive Message",
		URL:        receiveURL,
		Data:       msgHelloWorld,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)),
	},
	{
		Label:      "Receive Empty Message",
		URL:        receiveURL,
		Data:       msgEmpty,
		Status:     400,
		Response:   "no text or attachment",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)),
	},
	{
		Label:      "Receive First Photo Attachment",
		URL:        receiveURL,
		Data:       msgFirstPhotoAttachment,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://foo.bar/x-photo.jpg"},
	},
	{
		Label:      "Receive First Graffiti Attachment",
		URL:        receiveURL,
		Data:       msgFirstGraffitiAttachment,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://foo.bar/graffiti.png"},
	},
	{
		Label:      "Receive First Sticker Attachment",
		URL:        receiveURL,
		Data:       msgFirstStickerAttachment,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://foo.bar/128x128_sticker.png"},
	},
	{
		Label:      "Receive First Audio Attachment",
		URL:        receiveURL,
		Data:       msgFirstAudioAttachment,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://foo.bar/audio.mp3"},
	},
	{
		Label:      "Receive First Audio Attachment",
		URL:        receiveURL,
		Data:       msgFirstDocAttachment,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"https://foo.bar/doc.pdf"},
	},
	{
		Label:      "Receive Geolocation Attachment",
		URL:        receiveURL,
		Data:       msgGeolocationOnly,
		Status:     200,
		Response:   "ok",
		URN:        Sp("vk:123456"),
		ExternalID: Sp("1"),
		Date:       Tp(time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC)), Attachments: []string{"geo:-9.652278,-35.701095"},
	},
	{
		Label:    "Validate secret",
		URL:      receiveURL,
		Data:     eventWithSecret,
		Status:   200,
		Response: "no message or server verification event",
	},
	{
		Label:    "Invalidate secret",
		URL:      receiveURL,
		Data:     eventWithoutSecret,
		Status:   400,
		Response: "wrong secret key",
	},
	{
		Label:    "Verify server",
		URL:      receiveURL,
		Data:     eventServerVerification,
		Status:   200,
		Response: "a1b2c3",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func buildMockVKService(testCases []ChannelHandleTestCase) *httptest.Server {
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
	server := buildMockVKService([]ChannelHandleTestCase{})
	defer server.Close()

	handler := newHandler().(courier.URNDescriber)
	urn, _ :=
		urns.NewURNFromParts(urns.VKScheme, "123456789", "", "")
	data := map[string]string{"name": "John Doe"}

	describe, err := handler.DescribeURN(context.Background(), testChannels[0],
		urn)
	assert.Nil(t, err)
	assert.Equal(t,
		data, describe)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiBaseURL = s.URL
	URLPhotoUploadServer = s.URL + "/upload/photo"
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:      "Send simple message",
		Text:       "Simple message",
		URN:        "vk:123456789",
		Status:     "S",
		SendPrep:   setSendURL,
		ExternalID: "1",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=&message=Simple+message&random_id=10&user_id=123456789&v=5.103",
			}: {
				Status: 200,
				Body:   `{"response": 1}`,
			},
		},
	},
	{
		Label:       "Send photo attachment",
		Text:        "",
		URN:         "vk:123456789",
		Attachments: []string{"image/png:https://foo.bar/image.png"},
		Status:      "S",
		SendPrep:    setSendURL,
		ExternalID:  "1",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/upload/photo",
				BodyContains: `media body`,
			}: {
				Status: 200,
				Body:   `{"server": 109876, "photo": "...", "hash": "zxc987qwe"}`,
			},
			MockedRequest{
				Method:   "POST",
				Path:     actionSaveUploadedPhotoInfo,
				RawQuery: "access_token=token123xyz&hash=zxc987qwe&photo=...&server=109876&v=5.103",
			}: {
				Status: 200,
				Body:   `{"response": [{"id": 1, "owner_id": 1901234}]}`,
			},
			MockedRequest{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=photo1901234_1&message=&random_id=10&user_id=123456789&v=5.103",
			}: {
				Status: 200,
				Body:   `{"response": 1}`,
			},
		},
	},
	{
		Label:       "Send photo and another attachment type",
		Text:        "Attachments",
		URN:         "vk:123456789",
		Attachments: []string{"image/png:https://foo.bar/image.png", "audio/mp3:https://foo.bar/audio.mp3"},
		Status:      "S",
		SendPrep:    setSendURL,
		ExternalID:  "1",
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method:       "POST",
				Path:         "/upload/photo",
				BodyContains: `media body`,
			}: {
				Status: 200,
				Body:   `{"server": 109876, "photo": "...", "hash": "zxc987qwe"}`,
			},
			MockedRequest{
				Method:   "POST",
				Path:     actionSaveUploadedPhotoInfo,
				RawQuery: "access_token=token123xyz&hash=zxc987qwe&photo=...&server=109876&v=5.103",
			}: {
				Status: 200,
				Body:   `{"response": [{"id": 1, "owner_id": 1901234}]}`,
			},
			MockedRequest{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=photo1901234_1&message=Attachments" + url.QueryEscape("\n\nhttps://foo.bar/audio.mp3") + "&random_id=10&user_id=123456789&v=5.103",
			}: {
				Status: 200,
				Body:   `{"response": 1}`,
			},
		},
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.Attachments {
			prefix, _ := SplitAttachment(attachment)
			if mediaType := strings.Split(prefix, "/")[0]; mediaType != "image" {
				continue
			}
			mockedCase.Attachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestSendMsg(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media body"))
	}))
	mockedSendTestCases := mockAttachmentURLs(mediaServer, sendTestCases)
	RunChannelSendTestCases(t, testChannels[0], newHandler(), mockedSendTestCases, nil)
}
