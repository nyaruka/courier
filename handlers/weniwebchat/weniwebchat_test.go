package weniwebchat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

const channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"

var testChannels = []courier.Channel{
	courier.NewMockChannel(channelUUID, "WWC", "250788383383", "", map[string]interface{}{}),
}

// ReceiveMsg test

var receiveURL = fmt.Sprintf("/c/wwc/%s/receive", channelUUID)

const (
	textMsgTemplate = `
	{
		"type":"message",
		"from":%q,
		"message":{
			"type":"text",
			"timestamp":%q,
			"text":%q
		}
	}
	`

	imgMsgTemplate = `
	{
		"type":"message",
		"from":%q,
		"message":{
			"type":"image",
			"timestamp":%q,
			"media_url":%q,
			"caption":%q
		}
	}
	`

	locationMsgTemplate = `
	{
		"type":"message",
		"from":%q,
		"message":{
			"type":"location",
			"timestamp":%q,
			"latitude":%q,
			"longitude":%q
		}
	}
	`

	invalidMsgTemplate = `
	{
		"type":"foo",
		"from":"bar",
		"message": {
			"id":"000001",
			"type":"text",
			"timestamp":"1616586927"
		}
	}
	`
)

var testCases = []ChannelHandleTestCase{
	{
		Label:    "Receive Valid Text Msg",
		URL:      receiveURL,
		Data:     fmt.Sprintf(textMsgTemplate, "2345678", "1616586927", "Hello Test!"),
		Name:     Sp("2345678"),
		URN:      Sp("ext:2345678"),
		Text:     Sp("Hello Test!"),
		Status:   200,
		Response: "Accepted",
	},
	{
		Label:      "Receive Valid Media",
		URL:        receiveURL,
		Data:       fmt.Sprintf(imgMsgTemplate, "2345678", "1616586927", "https://link.to/image.png", "My Caption"),
		Name:       Sp("2345678"),
		URN:        Sp("ext:2345678"),
		Text:       Sp("My Caption"),
		Attachment: Sp("https://link.to/image.png"),
		Status:     200,
		Response:   "Accepted",
	},
	{
		Label:      "Receive Valid Location",
		URL:        receiveURL,
		Data:       fmt.Sprintf(locationMsgTemplate, "2345678", "1616586927", "-9.6996104", "-35.7794614"),
		Name:       Sp("2345678"),
		URN:        Sp("ext:2345678"),
		Attachment: Sp("geo:-9.6996104,-35.7794614"),
		Status:     200,
		Response:   "Accepted",
	},
	{
		Label:  "Receive Invalid JSON",
		URL:    receiveURL,
		Data:   "{}",
		Status: 400,
	},
	{
		Label:    "Receive Text Msg With Blank Message Text",
		URL:      receiveURL,
		Data:     fmt.Sprintf(textMsgTemplate, "2345678", "1616586927", ""),
		Status:   400,
		Response: "blank message, media or location",
	},
	{
		Label:    "Receive Invalid Timestamp",
		URL:      receiveURL,
		Data:     fmt.Sprintf(textMsgTemplate, "2345678", "foo", "Hello Test!"),
		Status:   400,
		Response: "invalid timestamp: foo",
	},
	{
		Label:    "Receive Invalid Message",
		URL:      receiveURL,
		Data:     invalidMsgTemplate,
		Status:   200,
		Response: "ignoring request, unknown message type",
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// SendMsg test

func prepareSendMsg(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*courier.MockChannel).SetConfig(courier.ConfigBaseURL, s.URL)
	timestamp = "1616700878"
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase

		for j, attachment := range testCase.Attachments {
			mockedCase.Attachments[j] = strings.Replace(attachment, "https://foo.bar", mediaServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

var sendTestCases = []ChannelSendTestCase{
	{
		Label:          "Plain Send",
		Text:           "Simple Message",
		URN:            "ext:371298371241",
		Status:         string(courier.MsgSent),
		Path:           "/send",
		Headers:        map[string]string{"Content-type": "application/json"},
		RequestBody:    `{"type":"message","to":"371298371241","from":"250788383383","message":{"type":"text","timestamp":"1616700878","text":"Simple Message"}}`,
		ResponseStatus: 200,
		SendPrep:       prepareSendMsg,
	},
	{
		Label:          "Unicode Send",
		Text:           "☺",
		URN:            "ext:371298371241",
		Status:         string(courier.MsgSent),
		Path:           "/send",
		Headers:        map[string]string{"Content-type": "application/json"},
		RequestBody:    `{"type":"message","to":"371298371241","from":"250788383383","message":{"type":"text","timestamp":"1616700878","text":"☺"}}`,
		ResponseStatus: 200,
		SendPrep:       prepareSendMsg,
	},
	{
		Label:       "invalid Text Send",
		Text:        "Error",
		URN:         "ext:371298371241",
		Status:      string(courier.MsgFailed),
		Path:        "/send",
		Headers:     map[string]string{"Content-type": "application/json"},
		RequestBody: `{"type":"message","to":"371298371241","from":"250788383383","message":{"type":"text","timestamp":"1616700878","text":"Error"}}`,
		SendPrep:    prepareSendMsg,
	},
	{
		Label: "Medias Send",
		Text:  "Medias",
		Attachments: []string{
			"audio/mp3:https://foo.bar/audio.mp3",
			"application/pdf:https://foo.bar/file.pdf",
			"image/jpg:https://foo.bar/image.jpg",
			"video/mp4:https://foo.bar/video.mp4",
		},
		URN:            "ext:371298371241",
		Status:         string(courier.MsgSent),
		ResponseStatus: 200,
		SendPrep:       prepareSendMsg,
	},
	{
		Label:          "Invalid Media Type Send",
		Text:           "Medias",
		Attachments:    []string{"foo/bar:https://foo.bar/foo.bar"},
		URN:            "ext:371298371241",
		Status:         string(courier.MsgFailed),
		ResponseStatus: 400,
		SendPrep:       prepareSendMsg,
	},
	{
		Label:       "Invalid Media Send",
		Text:        "Medias",
		Attachments: []string{"image/png:https://foo.bar/image.png"},
		URN:         "ext:371298371241",
		Status:      string(courier.MsgFailed),
		SendPrep:    prepareSendMsg,
	},
	{
		Label:          "No Timestamp Prepare",
		Text:           "No prepare",
		URN:            "ext:371298371241",
		Status:         string(courier.MsgSent),
		ResponseStatus: 200,
		SendPrep: func(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
			c.(*courier.MockChannel).SetConfig(courier.ConfigBaseURL, s.URL)
			timestamp = ""
		},
	},
}

func TestSending(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)

		res.Write([]byte("media bytes"))
	}))
	mockedSendTestCases := mockAttachmentURLs(mediaServer, sendTestCases)
	mediaServer.Close()

	RunChannelSendTestCases(t, testChannels[0], newHandler(), mockedSendTestCases, nil)
}
