package dialog360

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"D3C",
		"250788383383",
		"RW",
		map[string]interface{}{
			"auth_token": "the-auth-token",
			"base_url":   "https://waba-v2.360dialog.io",
		}),
}

var (
	d3CReceiveURL = "/c/d3c/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
)

var testCasesD3C = []ChannelHandleTestCase{
	{
		Label:                 "Receive Message WAC",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/helloWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Duplicate Valid Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/duplicateWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Voice Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/voiceWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp(""),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://waba-v2.360dialog.io/whatsapp_business/attachments/?mid=id_voice"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Button Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/buttonWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("No"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Document Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/documentWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("80skaraokesonglistartist"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://waba-v2.360dialog.io/whatsapp_business/attachments/?mid=id_document"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Image Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/imageWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://waba-v2.360dialog.io/whatsapp_business/attachments/?mid=id_image"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Video Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/videoWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://waba-v2.360dialog.io/whatsapp_business/attachments/?mid=id_video"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Audio Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/audioWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://waba-v2.360dialog.io/whatsapp_business/attachments/?mid=id_audio"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive Valid Location Message",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/locationWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:0.000000,1.000000"},
		ExpectedURN:          "whatsapp:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                "Receive Invalid JSON",
		URL:                  d3CReceiveURL,
		Data:                 "not json",
		ExpectedRespStatus:   400,
		ExpectedBodyContains: "unable to parse",
	},
	{
		Label:                "Receive Invalid FROM",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidFrom.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid whatsapp id",
	},
	{
		Label:                "Receive Invalid timestamp JSON",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidTimestamp.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid timestamp",
	},
	{
		Label:                 "Receive Message WAC with error message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/errorMsg.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		ExpectedErrors:        []*courier.ChannelError{courier.ErrorExternal("131051", "Unsupported message type")},
		NoInvalidChannelCheck: true,
	},
	{
		Label:                 "Receive error message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/errorErrors.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		ExpectedErrors:        []*courier.ChannelError{courier.ErrorExternal("0", "We were unable to authenticate the app user")},
		NoInvalidChannelCheck: true,
	},
	{
		Label:                "Receive Valid Status",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/validStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedMsgStatus:    "S",
		ExpectedExternalID:   "external_id",
	},
	{
		Label:                "Receive Valid Status with error message",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/errorStatus.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedMsgStatus:    "F",
		ExpectedExternalID:   "external_id",
		ExpectedErrors:       []*courier.ChannelError{courier.ErrorExternal("131014", "Request for url https://URL.jpg failed with error: 404 (Not Found)")},
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"unknown status: in_orbit"`,
	},
	{
		Label:                "Receive Ignore Status",
		URL:                  d3CReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/ignoreStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"ignoring status: deleted"`,
	},
	{
		Label:                 "Receive Valid Interactive Button Reply Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/buttonReplyWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Yes"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
	{
		Label:                 "Receive Valid Interactive List Reply Message",
		URL:                   d3CReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/listReplyWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Yes"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
	},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newWAHandler(courier.ChannelType("D3C"), "360Dialog"), testCasesD3C)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newWAHandler(courier.ChannelType("D3C"), "360Dialog"), testCasesD3C)
}

func TestBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()

	d3CHandler := &handler{NewBaseHandler(courier.ChannelType("D3C"), "360Dialog")}
	req, _ := d3CHandler.BuildAttachmentRequest(context.Background(), mb, testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "the-auth-token", req.Header.Get("D360-API-KEY"))

}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	c.(*test.MockChannel).SetConfig("base_url", s.URL)
}

var SendTestCasesD3C = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Simple Message","preview_url":false}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"☺","preview_url":false}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:          "Audio Send",
		MsgText:        "audio caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"audio caption","preview_url":false}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Document Send",
		MsgText:             "document caption",
		MsgURN:              "whatsapp:250788123123",
		MsgAttachments:      []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Image Send",
		MsgText:             "image caption",
		MsgURN:              "whatsapp:250788123123",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"image caption"}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Video Send",
		MsgText:             "video caption",
		MsgURN:              "whatsapp:250788123123",
		MsgAttachments:      []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Send",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": ["Chef", "tomorrow"]}}`),
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Country Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "eng-US",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Invalid Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "bnt",
		MsgMetadata:         json.RawMessage(`{"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive Button Message Send",
		MsgText:             "Interactive Button Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"BUTTON1"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive List Message Send",
		MsgText:             "Interactive List Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive List Message Send In Spanish",
		MsgText:             "Hola",
		MsgURN:              "whatsapp:250788123123",
		MsgLocale:           "spa",
		MsgQuickReplies:     []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Hola"},"action":{"button":"Menú","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive Button Message Send with image attachment",
		MsgText:             "Interactive Button Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"BUTTON1"},
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"image","image":{"link":"https://foo.bar/image.jpg"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive Button Message Send with video attachment",
		MsgText:             "Interactive Button Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"BUTTON1"},
		MsgAttachments:      []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"video","video":{"link":"https://foo.bar/video.mp4"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Interactive Button Message Send with document attachment",
		MsgText:             "Interactive Button Msg",
		MsgURN:              "whatsapp:250788123123",
		MsgQuickReplies:     []string{"BUTTON1"},
		MsgAttachments:      []string{"document/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"document","document":{"link":"https://foo.bar/document.pdf","filename":"document.pdf"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:           "Interactive Button Message Send with audio attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3"},
		MsgAttachments:  []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"ROW1"}},{"type":"reply","reply":{"id":"1","title":"ROW2"}},{"type":"reply","reply":{"id":"2","title":"ROW3"}}]}}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "157b5e14568e8",
		SendPrep:           setSendURL,
	},
	{
		Label:               "Link Sending",
		MsgText:             "Link Sending https://link.com",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Link Sending https://link.com","preview_url":true}}`,
		ExpectedRequestPath: "/messages",
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Error Bad JSON",
		MsgText:            "Error",
		MsgURN:             "whatsapp:250788123123",
		MockResponseBody:   `bad json`,
		MockResponseStatus: 403,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "whatsapp:250788123123",
		MockResponseBody:   `{ "error": {"message": "(#130429) Rate limit hit","code": 130429 }}`,
		MockResponseStatus: 403,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("130429", "(#130429) Rate limit hit")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var ChannelWAC = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "D3C", "12345_ID", "", map[string]interface{}{
		"auth_token": "the-auth-token",
		"base_url":   "https://waba-v2.360dialog.io",
	})
	checkRedacted := []string{"the-auth-token"}

	RunChannelSendTestCases(t, ChannelWAC, newWAHandler(courier.ChannelType("D3C"), "360Dialog"), SendTestCasesD3C, checkRedacted, nil)
}
func TestGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, languageInfo{"en", "Menu"}, getSupportedLanguage(courier.NilLocale))
	assert.Equal(t, languageInfo{"en", "Menu"}, getSupportedLanguage(courier.Locale("eng")))
	assert.Equal(t, languageInfo{"en_US", "Menu"}, getSupportedLanguage(courier.Locale("eng-US")))
	assert.Equal(t, languageInfo{"pt_PT", "Menu"}, getSupportedLanguage(courier.Locale("por")))
	assert.Equal(t, languageInfo{"pt_PT", "Menu"}, getSupportedLanguage(courier.Locale("por-PT")))
	assert.Equal(t, languageInfo{"pt_BR", "Menu"}, getSupportedLanguage(courier.Locale("por-BR")))
	assert.Equal(t, languageInfo{"fil", "Menu"}, getSupportedLanguage(courier.Locale("fil")))
	assert.Equal(t, languageInfo{"fr", "Menu"}, getSupportedLanguage(courier.Locale("fra-CA")))
	assert.Equal(t, languageInfo{"en", "Menu"}, getSupportedLanguage(courier.Locale("run")))
}
