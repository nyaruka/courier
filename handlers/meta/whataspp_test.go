package meta

import (
	"context"
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
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", []string{urns.WhatsApp.Prefix}, map[string]any{courier.ConfigAuthToken: "a123"}),
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
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Simple Message","preview_url":false}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Unicode Send",
		MsgText: "☺",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"☺","preview_url":false}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Audio Send",
		MsgText:        "audio caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"audio caption","preview_url":false}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:          "Document Send",
		MsgText:        "document caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption","filename":"document.pdf"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Image Send",
		MsgText:        "image caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg","caption":"image caption"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Video Send",
		MsgText:        "video caption",
		MsgURN:         "whatsapp:250788123123",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4","caption":"video caption"}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"type": "body", "name": "body", "variables": {"1": 0, "2": 1}}
			],
			"variables": [
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:          "Template Send with attachment",
		MsgText:        "templated message",
		MsgURN:         "whatsapp:250788123123",
		MsgLocale:      "eng",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/example.jpg"},
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"name": "header","type": "header/media", "variables": {"1": 0}},
				{"type": "body", "name": "body", "variables": {"1": 1, "2": 2}}
			],
			"variables": [
				{"type":"image", "value":"image/jpeg:https://foo.bar/image.jpg"},
				{"type": "text", "value": "Chef"},
				{"type": "text" , "value": "tomorrow"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send, no variables",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"},
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(200, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:     "Template Send, buttons params",
		MsgText:   "templated message",
		MsgURN:    "whatsapp:250788123123",
		MsgLocale: "eng",
		MsgTemplating: `{
			"template": {"uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3", "name": "revive_issue"}, 
			"components": [
				{"name": "header", "type": "header/media", "variables": {"1": 0}},
				{"name": "body", "type": "body/text", "variables": {"1": 1, "2": 2}},
				{"name": "button.0", "type": "button/quick_reply", "variables": {"1": 3}},
				{"name": "button.1", "type": "button/url", "variables": {"1": 4}}
			],
			"variables": [
				{"type": "image", "value": "image/jpeg:https://foo.bar/image.jpg"},
				{"type": "text", "value": "Ryan Lewis"},
				{"type": "text", "value": "niño"},
				{"type": "text", "value": "Sip"},
				{"type": "text", "value": "id00231"}
			],
			"language": "en_US"
		}`,
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"header","parameters":[{"type":"image","image":{"link":"https://foo.bar/image.jpg"}}]},{"type":"body","parameters":[{"type":"text","text":"Ryan Lewis"},{"type":"text","text":"niño"}]},{"type":"button","sub_type":"quick_reply","index":"0","parameters":[{"type":"payload","payload":"Sip"}]},{"type":"button","sub_type":"url","index":"1","parameters":[{"type":"text","text":"id00231"}]}]}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send, more than 10 QRs",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4", "ROW5", "ROW6", "ROW7", "ROW8", "ROW9", "ROW10", "ROW11", "ROW12"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"},{"id":"4","title":"ROW5"},{"id":"5","title":"ROW6"},{"id":"6","title":"ROW7"},{"id":"7","title":"ROW8"},{"id":"8","title":"ROW9"},{"id":"9","title":"ROW10"}]}]}}}`,
		}},
		ExpectedExtIDs:    []string{"157b5e14568e8"},
		ExpectedLogErrors: []*courier.ChannelError{courier.NewChannelError("", "", "too many quick replies WAC supports only up to 10 quick replies")},
	},
	{
		Label:           "Interactive List Message Send In Spanish",
		MsgText:         "Hola",
		MsgURN:          "whatsapp:250788123123",
		MsgLocale:       "spa",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Hola"},"action":{"button":"Menú","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		}},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send with image attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"image","image":{"link":"https://foo.bar/image.jpg"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send with video attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MsgAttachments:  []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"video","video":{"link":"https://foo.bar/video.mp4"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send with document attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
		MsgAttachments:  []string{"document/pdf:https://foo.bar/document.pdf"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","header":{"type":"document","document":{"link":"https://foo.bar/document.pdf","filename":"document.pdf"}},"body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:           "Interactive Button Message Send with audio attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3"},
		MsgAttachments:  []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"ROW1"}},{"type":"reply","reply":{"id":"1","title":"ROW2"}},{"type":"reply","reply":{"id":"2","title":"ROW3"}}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:           "Interactive List Message Send with attachment",
		MsgText:         "Interactive List Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		MsgAttachments:  []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`},
			{Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`},
		},
		ExpectedExtIDs: []string{"157b5e14568e8", "157b5e14568e8"},
	},
	{
		Label:   "Link Sending",
		MsgText: "Link Sending https://link.com",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(201, nil, []byte(`{ "messages": [{"id": "157b5e14568e8"}] }`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/12345_ID/messages",
				Body: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Link Sending https://link.com","preview_url":true}}`,
			},
		},
		ExpectedExtIDs: []string{"157b5e14568e8"},
	},
	{
		Label:   "Error Bad JSON",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(403, nil, []byte(`bad json`)),
			},
		},
		ExpectedError: courier.ErrResponseUnparseable,
	},
	{
		Label:   "Error",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(403, nil, []byte(`{ "error": {"message": "(#130429) Rate limit hit","code": 130429 }}`)),
			},
		},
		ExpectedError: courier.ErrFailedWithReason("130429", "(#130429) Rate limit hit"),
	},
	{
		Label:   "Error Connection",
		MsgText: "Error",
		MsgURN:  "whatsapp:250788123123",
		MockResponses: map[string][]*httpx.MockResponse{
			"*/12345_ID/messages": {
				httpx.NewMockResponse(500, nil, []byte(`Bad Gateway`)),
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestWhatsAppOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", []string{urns.WhatsApp.Prefix}, map[string]any{courier.ConfigAuthToken: "a123"})

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
	config := courier.NewDefaultConfig()
	config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"
	return courier.NewServer(config, backend)
}
