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
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "TWT", "tweeter", "",
		map[string]interface{}{
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

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: helloMsg, Status: 200, Response: "Accepted",
		Name: Sp("Nicolas Pottier"), URN: Sp("twitterid:272953809#nicpottier"),
		Text: Sp("Hello World & good wishes."), ExternalID: Sp("958501034212564996"), Date: Tp(time.Date(2018, 1, 31, 0, 43, 49, 301000000, time.UTC))},
	{Label: "Receive Attachment", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: attachment, Status: 200, Response: "Accepted",
		Text: Sp("Hello"), Attachments: []string{"https://image.foo.com/image.jpg"}, URN: Sp("twitterid:272953809#nicpottier"), ExternalID: Sp("958501034212564996"), Date: Tp(time.Date(2018, 1, 31, 0, 43, 49, 301000000, time.UTC))},
	{Label: "Not JSON", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: notJSON, Status: 400, Response: "Error"},
	{Label: "Invalid Twitter handle", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTwitterHandle, Status: 400, Response: "invalid twitter handle"},
	{Label: "Invalid Twitter ID", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Data: invalidTwitterID, Status: 400, Response: "invalid twitter id"},

	{Label: "Webhook Verification", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive?crc_token=test+token", Status: 200, Response: "sha256=O5hJl2njQRIa4vsumZ+3oom9ECR5m3aQLRZkPoYelp0="},
	{Label: "Webhook Verification Error", URL: "/c/twt/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive", Status: 400, Response: "missing required 'crc_token'"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler("TWT", "Twitter Activity"), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler("TWT", "Twitter Activity"), testCases)
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendDomain = s.URL
	uploadDomain = s.URL
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "twitterid:12345",
		Status: "W", ExternalID: "133",
		Path:         "/1.1/direct_messages/events/new.json",
		ResponseBody: `{"event": { "id": "133"}}`, ResponseStatus: 200,
		RequestBody: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Simple Message"}}}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "twitterid:12345", QuickReplies: []string{"Yes", "No, but a really long no that is unreasonably long"},
		Status: "W", ExternalID: "133",
		ResponseBody: `{"event": { "id": "133"}}`, ResponseStatus: 200,
		RequestBody: `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"Are you happy?","quick_reply":{"type":"options","options":[{"label":"Yes"},{"label":"No, but a really long no that is unr"}]}}}}}`,
		SendPrep:    setSendURL},
	{Label: "Image Send",
		Text:   "document caption",
		URN:    "twitterid:12345",
		Status: "W", ExternalID: "133",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_image&media_type=image%2Fjpeg&total_bytes=10`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Image Send",
		Text:   "document caption",
		URN:    "twitterid:12345",
		Status: "W", ExternalID: "133",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_image&media_type=image%2Fjpeg&total_bytes=10`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Video Send",
		Text:   "document caption",
		URN:    "twitterid:12345",
		Status: "W", ExternalID: "133",
		Attachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=INIT&media_category=dm_video&media_type=video%2Fmp4&total_bytes=10`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/1.1/media/upload.json",
				BodyContains: "APPEND",
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/media/upload.json",
				Body:   `command=FINALIZE&media_id=710511363345354753`,
			}: MockedResponse{
				Status: 200,
				Body: `{
					"media_id": 710511363345354753,
					"media_id_string": "710511363345354753",
					"processing_info" : {"state": "pending", "check_after_secs": 2},
				  }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"document caption"}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"","attachment":{"type":"media","media":{"id":"710511363345354753"}}}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
		},
		SendPrep: setSendURL,
	},
	{Label: "Send Audio",
		Text:   "My audio!",
		URN:    "twitterid:12345",
		Status: "W", ExternalID: "133",
		Attachments: []string{"audio/mp3:https://foo.bar/audio.mp3"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/1.1/direct_messages/events/new.json",
				Body:   `{"event":{"type":"message_create","message_create":{"target":{"recipient_id":"12345"},"message_data":{"text":"My audio!"}}}}`,
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
			MockedRequest{
				Method:       "POST",
				Path:         "/1.1/direct_messages/events/new.json",
				BodyContains: `"text":"http`, // audio link send as text
			}: MockedResponse{
				Status: 200,
				Body:   `{"event": { "id": "133"}}`,
			},
		},
		SendPrep: setSendURL},
	{Label: "ID Error",
		Text: "ID Error", URN: "twitterid:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error",
		Text: "Error", URN: "twitterid:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

func mockAttachmentURLs(mediaServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))
	for i, testCase := range testCases {
		mockedCase := testCase
		for j, attachment := range testCase.Attachments {
			parts := strings.SplitN(attachment, ":", 2)
			mimeType := parts[0]
			urlString := parts[1]
			parsedURL, _ := url.Parse(urlString)
			mockedCase.Attachments[j] = fmt.Sprintf("%s:%s%s", mimeType, mediaServer.URL, parsedURL.Path)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}
func TestSending(t *testing.T) {
	// fake media server that just replies with 200 and "media body" for content
	mediaServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		res.WriteHeader(200)
		res.Write([]byte("media body"))
	}))

	attachmentMockedSendTestCase := mockAttachmentURLs(mediaServer, defaultSendTestCases)
	RunChannelSendTestCases(t, testChannels[0], newHandler("TWT", "Twitter Activity"), attachmentMockedSendTestCase, nil)
}
