package vk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/vk/" + channelUUID + "/receive"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(
		channelUUID,
		"VK",
		"123456789",
		"",
		map[string]any{
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
const msgKeyboard = `{
	"type": "message_new",
	"object": {
	   "message": {
	   "id": 1,
       "date": 1580125800,
       "from_id": 123456,
       "text": "Yes",
	   "payload": "\"Yes\""
	  }
	},
	"secret": "abc123xyz"
  }`

const keyboardJson = `{"one_time":true,"buttons":[[{"action":{"type":"text","label":"A","payload":"\"A\""},"color":"primary"},{"action":{"type":"text","label":"B","payload":"\"B\""},"color":"primary"},{"action":{"type":"text","label":"C","payload":"\"C\""},"color":"primary"},{"action":{"type":"text","label":"D","payload":"\"D\""},"color":"primary"},{"action":{"type":"text","label":"E","payload":"\"E\""},"color":"primary"}]],"inline":false}`

var testCases = []IncomingTestCase{
	{
		Label:                "Receive Message",
		URL:                  receiveURL,
		Data:                 msgHelloWorld,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
	},
	{
		Label:                "Receive Empty Message",
		URL:                  receiveURL,
		Data:                 msgEmpty,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "no text or attachment",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
	},
	{
		Label:                "Receive First Photo Attachment",
		URL:                  receiveURL,
		Data:                 msgFirstPhotoAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"https://foo.bar/x-photo.jpg"},
	},
	{
		Label:                "Receive First Graffiti Attachment",
		URL:                  receiveURL,
		Data:                 msgFirstGraffitiAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"https://foo.bar/graffiti.png"},
	},
	{
		Label:                "Receive First Sticker Attachment",
		URL:                  receiveURL,
		Data:                 msgFirstStickerAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"https://foo.bar/128x128_sticker.png"},
	},
	{
		Label:                "Receive First Audio Attachment",
		URL:                  receiveURL,
		Data:                 msgFirstAudioAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"https://foo.bar/audio.mp3"},
	},
	{
		Label:                "Receive First Audio Attachment",
		URL:                  receiveURL,
		Data:                 msgFirstDocAttachment,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"https://foo.bar/doc.pdf"},
	},
	{
		Label:                "Receive Message Keyboard",
		URL:                  receiveURL,
		Data:                 msgKeyboard,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedMsgText:      Sp("Yes"),
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
	},
	{
		Label:                "Receive Geolocation Attachment",
		URL:                  receiveURL,
		Data:                 msgGeolocationOnly,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "ok",
		ExpectedURN:          "vk:123456",
		ExpectedExternalID:   "1",
		ExpectedDate:         time.Date(2020, 1, 27, 11, 50, 0, 0, time.UTC),
		ExpectedAttachments:  []string{"geo:-9.652278,-35.701095"},
	},
	{
		Label:                "Valid secret",
		URL:                  receiveURL,
		Data:                 `{"type": "some_event", "object": {}, "secret": "abc123xyz"}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no message or server verification event",
	},
	{
		Label:                "Missing secret",
		URL:                  receiveURL,
		Data:                 `{"type": "some_event", "object": {}}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "Field validation for 'SecretKey' failed on the 'required' tag",
	},
	{
		Label:                "Invalid secret",
		URL:                  receiveURL,
		Data:                 `{"type": "some_event", "object": {}, "secret": "0987654321"}`,
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "wrong secret key",
	},
	{
		Label:                "Verify server",
		URL:                  receiveURL,
		Data:                 `{"type": "confirmation", "secret": "abc123xyz"}`,
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "a1b2c3",
	},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler(), testCases)
}

func buildMockVKService(testCases []IncomingTestCase) *httptest.Server {
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

func TestDescribeURN(t *testing.T) {
	server := buildMockVKService([]IncomingTestCase{})
	defer server.Close()

	handler := newHandler()
	handler.Initialize(test.NewMockServer(courier.NewConfig(), test.NewMockBackend()))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, testChannels[0], handler.RedactValues(testChannels[0]))
	urn, _ := urns.NewURNFromParts(urns.VKScheme, "123456789", "", "")
	data := map[string]string{"name": "John Doe"}

	describe, err := handler.(courier.URNDescriber).DescribeURN(context.Background(), testChannels[0], urn, clog)
	assert.Nil(t, err)
	assert.Equal(t, data, describe)

	AssertChannelLogRedaction(t, clog, []string{"token123xyz", "abc123xyz"})
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	apiBaseURL = s.URL
	URLPhotoUploadServer = s.URL + "/upload/photo"
}

var sendTestCases = []OutgoingTestCase{
	{
		Label:              "Send simple message",
		MsgText:            "Simple message",
		MsgURN:             "vk:123456789",
		ExpectedMsgStatus:  "S",
		SendPrep:           setSendURL,
		ExpectedExternalID: "1",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=&message=Simple+message&random_id=10&user_id=123456789&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": 1}`)),
		},
	},
	{
		Label:              "Send photo attachment",
		MsgText:            "",
		MsgURN:             "vk:123456789",
		MsgAttachments:     []string{"image/png:https://foo.bar/image.png"},
		ExpectedMsgStatus:  "S",
		SendPrep:           setSendURL,
		ExpectedExternalID: "1",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/upload/photo",
				BodyContains: `media body`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"server": 109876, "photo": "...", "hash": "zxc987qwe"}`)),
			{
				Method:   "POST",
				Path:     actionSaveUploadedPhotoInfo,
				RawQuery: "access_token=token123xyz&hash=zxc987qwe&photo=...&server=109876&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": [{"id": 1, "owner_id": 1901234}]}`)),
			{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=photo1901234_1&message=&random_id=10&user_id=123456789&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": 1}`)),
		},
	},
	{
		Label:              "Send photo and another attachment type",
		MsgText:            "Attachments",
		MsgURN:             "vk:123456789",
		MsgAttachments:     []string{"image/png:https://foo.bar/image.png", "audio/mp3:https://foo.bar/audio.mp3"},
		ExpectedMsgStatus:  "S",
		SendPrep:           setSendURL,
		ExpectedExternalID: "1",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/upload/photo",
				BodyContains: `media body`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"server": 109876, "photo": "...", "hash": "zxc987qwe"}`)),
			{
				Method:   "POST",
				Path:     actionSaveUploadedPhotoInfo,
				RawQuery: "access_token=token123xyz&hash=zxc987qwe&photo=...&server=109876&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": [{"id": 1, "owner_id": 1901234}]}`)),
			{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=photo1901234_1&message=Attachments" + url.QueryEscape("\n\nhttps://foo.bar/audio.mp3") + "&random_id=10&user_id=123456789&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": 1}`)),
		},
	},
	{
		Label:              "Send keyboard",
		MsgText:            "Send keyboard",
		MsgURN:             "vk:123456789",
		MsgQuickReplies:    []string{"A", "B", "C", "D", "E"},
		ExpectedMsgStatus:  "S",
		SendPrep:           setSendURL,
		ExpectedExternalID: "1",
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:   "POST",
				Path:     actionSendMessage,
				RawQuery: "access_token=token123xyz&attachment=&keyboard=" + url.QueryEscape(keyboardJson) + "&message=Send+keyboard&random_id=10&user_id=123456789&v=5.103",
			}: httpx.NewMockResponse(200, nil, []byte(`{"response": 1}`)),
		},
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []OutgoingTestCase) []OutgoingTestCase {
	casesWithMockedUrls := make([]OutgoingTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.MsgAttachments {
			prefix, _ := SplitAttachment(attachment)
			if mediaType := strings.Split(prefix, "/")[0]; mediaType != "image" {
				continue
			}
			mockedCase.MsgAttachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestSend(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media body"))
	}))
	mockedSendTestCases := mockAttachmentURLs(mediaServer, sendTestCases)
	RunOutgoingTestCases(t, testChannels[0], newHandler(), mockedSendTestCases, []string{"token123xyz", "abc123xyz"}, nil)
}
