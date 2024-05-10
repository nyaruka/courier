package twitter

import (
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TWT", "tweeter", "",
		[]string{urns.Twitter.Prefix, urns.TwitterID.Prefix},
		map[string]any{
			configHandleID:          "835740314006511618",
			configAPIKey:            "apiKey",
			configAPISecret:         "apiSecret",
			configAccessToken:       "accessToken",
			configAccessTokenSecret: "accessTokenSecret",
		}),
}

var helloMsg = `{
	"direct_message_events": [
			{
					"type": "message_create",
					"id": "958501034212564996",
					"created_timestamp": "1517359429301",
					"message_create": {
							"target": {
									"recipient_id": "835740314006511618"
							},
							"sender_id": "272953809",
							"message_data": {
									"text": "Hello World &amp; good wishes."
							}
					}
			}
	],
	"users": {
			"272953809": {
					"id": "272953809",
					"created_timestamp": "1301236201000",
					"name": "Nicolas Pottier",
					"screen_name": "nicpottier"
			},
			"835740314006511618": {
					"id": "835740314006511618",
					"created_timestamp": "1488090992969",
					"name": "Resistbot",
					"screen_name": "resistbot"
			}
	}
}`

var invalidTwitterID = `{
	"direct_message_events": [
			{
					"type": "message_create",
					"id": "958501034212564996",
					"created_timestamp": "1517359429301",
					"message_create": {
							"target": {
									"recipient_id": "835740314006511618"
							},
							"sender_id": "272953qwe809",
							"message_data": {
									"text": "Hello World"
							}
					}
			}
	],
	"users": {
			"272953qwe809": {
					"id": "272953qwe809",
					"created_timestamp": "1301236201000",
					"name": "Nicolas Pottier",
					"screen_name": "nicpottier"
			},
			"835740314006511618": {
					"id": "835740314006511618",
					"created_timestamp": "1488090992969",
					"name": "Resistbot",
					"screen_name": "resistbot"
			}
	}
}`

var attachment = `{
	"direct_message_events": [
			{
					"type": "message_create",
					"id": "958501034212564996",
					"created_timestamp": "1517359429301",
					"message_create": {
							"target": {
									"recipient_id": "835740314006511618"
							},
							"sender_id": "272953809",
							"message_data": {
									"text": "Hello",
									"attachment": {
									"type": "media",
										"media": {
											"media_url_https": "https://image.foo.com/image.jpg"
										}
									}
							}
					}
			}
	],
	"users": {
			"272953809": {
					"id": "272953809",
					"created_timestamp": "1301236201000",
					"name": "Nicolas Pottier",
					"screen_name": "nicpottier"
			},
			"835740314006511618": {
					"id": "835740314006511618",
					"created_timestamp": "1488090992969",
					"name": "Resistbot",
					"screen_name": "resistbot"
			}
	}
}`

var notJSON = `blargh`

var testCases = []IncomingTestCase{
	{Label: "Receive Message", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedContactName: Sp("Nicolas Pottier"), ExpectedURN: "twitterid:272953809#nicpottier",
		ExpectedMsgText: Sp("Hello World & good wishes."), ExpectedExternalID: "958501034212564996", ExpectedDate: time.Date(2018, 1, 31, 0, 43, 49, 301000000, time.UTC)},
	{Label: "Receive Attachment", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: attachment, ExpectedRespStatus: 200, ExpectedBodyContains: "Accepted",
		ExpectedMsgText: Sp("Hello"), ExpectedAttachments: []string{"https://image.foo.com/image.jpg"}, ExpectedURN: "twitterid:272953809#nicpottier", ExpectedExternalID: "958501034212564996", ExpectedDate: time.Date(2018, 1, 31, 0, 43, 49, 301000000, time.UTC)},
	{Label: "Not JSON", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: notJSON, ExpectedRespStatus: 400, ExpectedBodyContains: "Error"},
	{Label: "Invalid Twitter ID", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTwitterID, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid twitter id"},

	{Label: "Webhook Verification", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?crc_token=test+token", ExpectedRespStatus: 200, ExpectedBodyContains: "sha256=O5hJl2njQRIa4vsumZ+3oom9ECR5m3aQLRZkPoYelp0="},
	{Label: "Webhook Verification Error", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", ExpectedRespStatus: 400, ExpectedBodyContains: "missing required 'crc_token'"},
}

func TestIncoming(t *testing.T) {
	RunIncomingTestCases(t, testChannels, newHandler("TWT", "Twitter Activity"), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("TWT", "Twitter Activity"), testCases)
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:   "Plain Send",
		MsgText: "Simple Message",
		MsgURN:  "twitterid:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twitter.com/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{
				Path: "/1.1/direct_messages/events/new.json",
				Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Simple Message"}}}}`,
			},
		},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:           "Quick Reply",
		MsgText:         "Are you happy?",
		MsgURN:          "twitterid:12345",
		MsgQuickReplies: []string{"Yes", "No, but a really long no that is unreasonably long"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twitter.com/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{{
			Path: "/1.1/direct_messages/events/new.json",
			Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Are you happy?","quick_reply":{"type":"options","options":[{"label":"Yes"},{"label":"No, but a really long no that is unr"}]}}}}}`,
		}},
		ExpectedExtIDs: []string{"133"},
	},
	{
		Label:          "Image Send",
		MsgText:        "document caption",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/image.jpg": {
				httpx.NewMockResponse(200, nil, []byte(`media body`)),
			},
			"*/1.1/media/upload.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			},
			"*/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`},
			{},
			{Body: `command=INIT&media_category=dm_image&media_type=image%2Fjpeg&total_bytes=10`},
			{BodyContains: "APPEND"},
			{Body: `command=FINALIZE&media_id=710511363345354753`},
			{Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`},
		},
		ExpectedExtIDs: []string{"133", "133"},
	},
	{
		Label:          "Video Send",
		MsgText:        "document caption",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[string][]*httpx.MockResponse{
			"https://foo.bar/video.mp4": {
				httpx.NewMockResponse(200, nil, []byte(`media body`)),
			},
			"*/1.1/media/upload.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			},
			"*/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`},
			{},
			{Body: `command=INIT&media_category=dm_video&media_type=video%2Fmp4&total_bytes=10`},
			{BodyContains: "APPEND"},
			{Body: `command=FINALIZE&media_id=710511363345354753`},
			{Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`},
		},
		ExpectedExtIDs: []string{"133", "133"},
	},
	{
		Label:          "Send Audio",
		MsgText:        "My audio!",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[string][]*httpx.MockResponse{
			"*/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
				httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			},
		},
		ExpectedRequests: []ExpectedRequest{
			{Body: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"My audio!"}}}}`},
			{BodyContains: `"text":"http`}, // audio link send as text
		},
		ExpectedExtIDs:    []string{"133", "133"},
		ExpectedLogErrors: []*courier.ChannelError{courier.NewChannelError("", "", "unable to upload media, unsupported Twitter attachment")},
	},
	{
		Label:   "ID Error",
		MsgText: "ID Error",
		MsgURN:  "twitterid:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twitter.com/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(200, nil, []byte(`{ "is_error": true }`)),
			},
		},
		ExpectedError:     courier.ErrResponseUnexpected,
		ExpectedLogErrors: []*courier.ChannelError{courier.NewChannelError("", "", "unable to get message_id from body")},
	},
	{
		Label:   "Error",
		MsgText: "Error",
		MsgURN:  "twitterid:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twitter.com/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(403, nil, []byte(`{ "is_error": true }`)),
			},
		},
		ExpectedError: courier.ErrResponseStatus,
	},
	{
		Label:   "Connection Error",
		MsgText: "Error",
		MsgURN:  "twitterid:12345",
		MockResponses: map[string][]*httpx.MockResponse{
			"https://api.twitter.com/1.1/direct_messages/events/new.json": {
				httpx.NewMockResponse(500, nil, []byte(`{ "is_error": true }`)),
			},
		},
		ExpectedError: courier.ErrConnectionFailed,
	},
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTestCases(t, testChannels[0], newHandler("TWT", "Twitter Activity"), defaultSendTestCases, []string{"apiSecret", "accessTokenSecret"}, nil)
}
