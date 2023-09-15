package meta

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var whatsappTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", map[string]any{courier.ConfigAuthToken: "a123"}),
}

var whatappReceiveURL = "/c/wac/receive"

var whatsappIncomingTests = []IncomingTestCase{
	{
		Label:                 "Receive Message WAC",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/hello.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Duplicate Valid Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/duplicate.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Voice Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/voice.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp(""),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://foo.bar/attachmentURL_Voice"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Button Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/button.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("No"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Document Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/document.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("80skaraokesonglistartist"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://foo.bar/attachmentURL_Document"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Image Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/image.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://foo.bar/attachmentURL_Image"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Video Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/video.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://foo.bar/attachmentURL_Video"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Audio Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/audio.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Check out my new phone!"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedAttachments:   []string{"https://foo.bar/attachmentURL_Audio"},
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Valid Location Message",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/location.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"msg"`,
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:0.000000,1.000000"},
		ExpectedURN:          "whatsapp:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid JSON",
		URL:                  whatappReceiveURL,
		Data:                 "not json",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid From",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalid_from.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid whatsapp id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid Timestamp",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalid_timestamp.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid timestamp",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                 "Receive Message WAC invalid signature",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/hello.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "invalid request signature",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		PrepRequest:           addInvalidSignature,
	},
	{
		Label:                 "Receive Message WAC with error message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/error_msg.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		ExpectedErrors:        []*courier.ChannelError{courier.ErrorExternal("131051", "Unsupported message type")},
		NoInvalidChannelCheck: true,
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive error message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/error_errors.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		ExpectedErrors:        []*courier.ChannelError{courier.ErrorExternal("0", "We were unable to authenticate the app user")},
		NoInvalidChannelCheck: true,
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Valid Status",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/valid_status.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external_id", Status: courier.MsgStatusSent},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Valid Status with error message",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/error_status.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedStatuses: []ExpectedStatus{
			{ExternalID: "external_id", Status: courier.MsgStatusFailed},
		},
		ExpectedErrors: []*courier.ChannelError{
			courier.ErrorExternal("131014", "Request for url https://URL.jpg failed with error: 404 (Not Found)"),
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalid_status.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"unknown status: in_orbit"`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Ignore Status",
		URL:                  whatappReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/ignore_status.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"ignoring status: deleted"`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                 "Receive Valid Interactive Button Reply Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/button_reply.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Yes"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                 "Receive Valid Interactive List Reply Message",
		URL:                   whatappReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/list_reply.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Yes"),
		ExpectedURN:           "whatsapp:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC),
		PrepRequest:           addValidSignature,
	},
}

func TestWhatsAppIncoming(t *testing.T) {
	graphURL = createMockGraphAPI().URL

	RunIncomingTestCases(t, whatsappTestChannels, newHandler("WAC", "Cloud API WhatsApp"), whatsappIncomingTests)
}

var whatsappOutgoingTests = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "whatsapp:250788123123",
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  201,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Simple Message","preview_url":false}}`,
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/12345_ID/messages",
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
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			{
				Method: "POST",
				Path:   "/12345_ID/messages",
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
		ExpectedRequestPath: "/12345_ID/messages",
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

func TestWhatsAppOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", map[string]any{courier.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTestCases(t, channel, newHandler("WAC", "Cloud API WhatsApp"), whatsappOutgoingTests, checkRedacted, nil)
}

func TestWhatsAppDescribeURN(t *testing.T) {
	channel := whatsappTestChannels[0]
	handler := newHandler("WAC", "Cloud API WhatsApp")
	handler.Initialize(newServerWithWAC(nil))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"whatsapp:1337", map[string]string{}},
		{"whatsapp:4567", map[string]string{}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(courier.URNDescriber).DescribeURN(context.Background(), whatsappTestChannels[0], tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestWhatsAppBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()
	s := newServerWithWAC(mb)
	handler := &handler{NewBaseHandler(courier.ChannelType("WAC"), "WhatsApp Cloud", DisableUUIDRouting())}
	handler.Initialize(s)
	req, _ := handler.BuildAttachmentRequest(context.Background(), mb, whatsappTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer wac_admin_system_user_token", req.Header.Get("Authorization"))
}

func newServerWithWAC(backend courier.Backend) courier.Server {
	config := courier.NewConfig()
	config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"
	return courier.NewServer(config, backend)
}
