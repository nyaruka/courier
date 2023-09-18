package twitter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
)

var testChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TWT", "tweeter", "",
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

var invalidTwitterHandle = `{
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
									"text": "Hello World"
							}
					}
			}
	],
	"users": {
			"272953809": {
					"id": "272953809",
					"created_timestamp": "1301236201000",
					"name": "Nicolas Pottier",
					"screen_name": "nicpottier!!$"
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
	{Label: "Invalid Twitter handle", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTwitterHandle, ExpectedRespStatus: 400, ExpectedBodyContains: "invalid twitter handle"},
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

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendDomain = s.URL
	uploadDomain = s.URL
}

var defaultSendTestCases = []OutgoingTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "twitterid:12345",
		MockResponseBody:    `{"event": { "id": "133"}}`,
		MockResponseStatus:  200,
		ExpectedRequestPath: "/1.1/direct_messages/events/new.json",
		ExpectedRequestBody: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Simple Message"}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "twitterid:12345",
		MsgQuickReplies:     []string{"Yes", "No, but a really long no that is unreasonably long"},
		MockResponseBody:    `{"event": { "id": "133"}}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Are you happy?","quick_reply":{"type":"options","options":[{"label":"Yes"},{"label":"No, but a really long no that is unr"}]}}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "133",
		SendPrep:            setSendURL,
	},
	{
		Label:          "Image Send",
		MsgText:        "document caption",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_image&media_type=image%2Fjpeg&total_bytes=10`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Image Send",
		MsgText:        "document caption",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_image&media_type=image%2Fjpeg&total_bytes=10`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Video Send",
		MsgText:        "document caption",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_video&media_type=video%2Fmp4&total_bytes=10`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"media_id": 710511363345354753, "media_id_string": "710511363345354753"}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		SendPrep:           setSendURL,
	},
	{
		Label:          "Send Audio",
		MsgText:        "My audio!",
		MsgURN:         "twitterid:12345",
		MsgAttachments: []string{"audio/mp3:https://foo.bar/audio.mp3"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"My audio!"}}}}`,
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
			{
				Method:       "POST",
				Path:         "/1.1/direct_messages/events/new.json",
				BodyContains: `"text":"http`, // audio link send as text
			}: httpx.NewMockResponse(200, nil, []byte(`{"event": { "id": "133"}}`)),
		},
		ExpectedMsgStatus:  "W",
		ExpectedExternalID: "133",
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "unable to upload media, unsupported Twitter attachment")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "ID Error",
		MsgText:            "ID Error",
		MsgURN:             "twitterid:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.NewChannelError("", "", "unable to get message_id from body")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "twitterid:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []OutgoingTestCase) []OutgoingTestCase {
	casesWithMockedUrls := make([]OutgoingTestCase, len(testCases))
	for i, testCase := range testCases {
		mockedCase := testCase
		for j, attachment := range testCase.MsgAttachments {
			parts := strings.SplitN(attachment, ":", 2)
			mimeType := parts[0]
			urlString := parts[1]
			parsedURL, _ := url.Parse(urlString)
			mockedCase.MsgAttachments[j] = fmt.Sprintf("%s:%s%s", mimeType, mediaServer.URL, parsedURL.Path)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}
func TestOutgoing(t *testing.T) {
	// fake media server that just replies with 200 and "media body" for content
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media body"))
	}))

	attachmentMockedSendTestCase := mockAttachmentURLs(mediaServer, defaultSendTestCases)
	RunOutgoingTestCases(t, testChannels[0], newHandler("TWT", "Twitter Activity"), attachmentMockedSendTestCase, []string{"apiSecret", "accessTokenSecret"}, nil)
}
