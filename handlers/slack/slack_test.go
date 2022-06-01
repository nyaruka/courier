package slack

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/sl/" + channelUUID + "/receive/"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel(channelUUID, "SL", "2022", "US", map[string]interface{}{"bot_token": "xoxb-abc123", "verification_token": "one-long-verification-token"}),
}

const helloMsg = `{
	"token": "one-long-verification-token",
	"team_id": "T061EG9R6",
	"api_app_id": "A0PNCHHK2",
	"event": {
			"type": "message",
			"channel": "C0123ABCDEF",
			"user": "U0123ABCDEF",
			"text": "Hello World!",
			"ts": "1355517523.000005",
			"event_ts": "1355517523.000005",
			"channel_type": "channel"
	},
	"type": "event_callback",
	"authed_teams": [
			"T061EG9R6"
	],
	"event_id": "Ev0PV52K21",
	"event_time": 1355517523
}`

const imageFileMsg = `{
	"token": "Bwf82iq5kCEkHOzRQ7p4FqkQ",
	"team_id": "T03CN5KTA6S",
	"api_app_id": "A03FTC8MZ63",
	"event": {
			"type": "message",
			"text": "",
			"files": [
					{
							"id": "F03GTH43SSF",
							"created": 1653417049,
							"timestamp": 1653417049,
							"name": "batata.jpg",
							"title": "batata.jpg",
							"mimetype": "image/jpeg",
							"filetype": "jpg",
							"pretty_type": "JPEG",
							"user": "U0123ABCDEF",
							"editable": false,
							"size": 7130,
							"mode": "hosted",
							"is_external": false,
							"external_type": "",
							"is_public": true,
							"public_url_shared": false,
							"display_as_bot": false,
							"username": "",
							"url_private": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GTH43SSF/batata.jpg",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GTH43SSF/download/batata.jpg",
							"media_display_type": "unknown",
							"thumb_64": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GTH43SSF-75138e6784/batata_64.jpg",
							"thumb_80": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GTH43SSF-75138e6784/batata_80.jpg",
							"thumb_360": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GTH43SSF-75138e6784/batata_360.jpg",
							"thumb_360_w": 360,
							"thumb_360_h": 360,
							"thumb_160": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GTH43SSF-75138e6784/batata_160.jpg",
							"original_w": 400,
							"original_h": 400,
							"thumb_tiny": "AwAwADDTooooAKKY7Y4HWmjnqalysOxLRTUJJxTqadxBTS4HvRIcLURIFTKVhpDZJhu6UAse1IuCSR16U/NZPXUvYWNyOq81MDkZFQk9xT4+5rSL6EseRkYNRtCD0JFSUVbSZJWW3dGzvz7YpTuB5VvwFWKKTgh3IQpYdxUoGBS0U0rA2f/Z",
							"permalink": "https://teste-apigrupo.slack.com/files/U0123ABCDEF/F03GTH43SSF/batata.jpg",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GTH43SSF-39fcf577f2",
							"has_rich_preview": false
					}
			],
			"upload": false,
			"user": "U0123ABCDEF",
			"display_as_bot": false,
			"ts": "1653417052.881009",
			"client_msg_id": "0e400b8f-07c4-452f-a13e-2744fcae2558",
			"channel": "C0123ABCDEF",
			"subtype": "file_share",
			"event_ts": "1653417052.881009",
			"channel_type": "channel"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653417052,
	"authorizations": [
			{
					"enterprise_id": null,
					"team_id": "T03CN5KTA6S",
					"user_id": "U03G81FQM98",
					"is_bot": true,
					"is_enterprise_install": false
			}
	],
	"is_ext_shared_channel": false,
	"event_context": "4-eyJldCI6Im1lc3NhZ2UiLCJ0aWQiOiJUMDNDTjVLVEE2UyIsImFpZCI6IkEwM0ZUQzhNWjYzIiwiY2lkIjoiQzAzQ1VRUUJIRUYifQ"
}
`

const audioFileMsg = `{
	"token": "Bwf82iq5kCEkHOzRQ7p4FqkQ",
	"team_id": "T03CN5KTA6S",
	"api_app_id": "A03FTC8MZ63",
	"event": {
			"type": "message",
			"text": "",
			"files": [
					{
							"id": "F03GWURCZL4",
							"created": 1653428828,
							"timestamp": 1653428828,
							"name": "here we go again.mp3",
							"title": "here we go again.mp3",
							"mimetype": "audio/mpeg",
							"filetype": "mp3",
							"pretty_type": "MP3",
							"user": "U0123ABCDEF",
							"editable": false,
							"size": 102122,
							"mode": "hosted",
							"is_external": false,
							"external_type": "",
							"is_public": true,
							"public_url_shared": false,
							"display_as_bot": false,
							"username": "",
							"transcription": {
									"status": "none"
							},
							"url_private": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GWURCZL4-9aaa1171c6/here_we_go_again_audio.mp4",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GWURCZL4/download/here_we_go_again.mp3",
							"duration_ms": 3187,
							"aac": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GWURCZL4-9aaa1171c6/here_we_go_again_audio.mp4",
							"audio_wave_samples": [0,0,0,3,5,2,3,6,9,10,9,8,5,3,9,19,22,23,24,25,25,30,28,13,10,10,8,7,4,1,5,7,9,10,9,12,56,35,59,15,10,10,8,6,3,3,6,8,10,10,8,5,2,3,6,8,10,10,22,50,42,20,59,73,30,14,26,67,65,72,72,86,94,36,12,42,100,91,91,86,54,40,23,15,7,5,8,9,10,8,6,3,2,5,8,10,9,8,5,2],
							"media_display_type": "audio",
							"permalink": "https://teste-apigrupo.slack.com/files/U0123ABCDEF/F03GWURCZL4/here_we_go_again.mp3",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GWURCZL4-471020b300",
							"has_rich_preview": false
					}
			],
			"upload": false,
			"user": "U0123ABCDEF",
			"display_as_bot": false,
			"ts": "1653428835.192419",
			"client_msg_id": "c827a681-2641-44ed-8cd5-854339499a1e",
			"channel": "C0123ABCDEF",
			"subtype": "file_share",
			"event_ts": "1653428835.192419",
			"channel_type": "channel"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653428835,
	"authorizations": [
			{
					"enterprise_id": null,
					"team_id": "T03CN5KTA6S",
					"user_id": "U03G81FQM98",
					"is_bot": true,
					"is_enterprise_install": false
			}
	],
	"is_ext_shared_channel": false,
	"event_context": "4-eyJldCI6Im1lc3NhZ2UiLCJ0aWQiOiJUMDNDTjVLVEE2UyIsImFpZCI6IkEwM0ZUQzhNWjYzIiwiY2lkIjoiQzAzQ1VRUUJIRUYifQ"
}
`

const videoFileMsg = `{
	"token": "Bwf82iq5kCEkHOzRQ7p4FqkQ",
	"team_id": "T03CN5KTA6S",
	"api_app_id": "A03FTC8MZ63",
	"event": {
			"type": "message",
			"text": "",
			"files": [
					{
							"id": "F03GDSSMC79",
							"created": 1653427226,
							"timestamp": 1653427226,
							"name": "Walk Cycle Animation sample.mp4",
							"title": "Walk Cycle Animation sample.mp4",
							"mimetype": "video/mp4",
							"filetype": "mp4",
							"pretty_type": "MPEG 4 Video",
							"user": "U0123ABCDEF",
							"editable": false,
							"size": 767148,
							"mode": "hosted",
							"is_external": false,
							"external_type": "",
							"is_public": true,
							"public_url_shared": false,
							"display_as_bot": false,
							"username": "",
							"transcription": {
									"status": "none"
							},
							"mp4": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GDSSMC79-0af57254d8/walk_cycle_animation_sample.mp4",
							"url_private": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GDSSMC79-0af57254d8/walk_cycle_animation_sample.mp4",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GDSSMC79/download/walk_cycle_animation_sample.mp4",
							"hls": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GDSSMC79-0af57254d8/file.m3u8",
							"duration_ms": 19953,
							"media_display_type": "video",
							"thumb_video": "https://files.slack.com/files-tmb/T03CN5KTA6S-F03GDSSMC79-0af57254d8/walk_cycle_animation_sample_thumb_video.jpeg",
							"thumb_video_w": 640,
							"thumb_video_h": 360,
							"permalink": "https://teste-apigrupo.slack.com/files/U0123ABCDEF/F03GDSSMC79/walk_cycle_animation_sample.mp4",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GDSSMC79-805aa1d85f",
							"has_rich_preview": false
					}
			],
			"upload": false,
			"user": "U0123ABCDEF",
			"display_as_bot": false,
			"ts": "1653427243.620839",
			"client_msg_id": "72df394d-bfbb-4d90-8db4-cbe5caa76b28",
			"channel": "C0123ABCDEF",
			"subtype": "file_share",
			"event_ts": "1653427243.620839",
			"channel_type": "channel"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653427243,
	"authorizations": [
			{
					"enterprise_id": null,
					"team_id": "T03CN5KTA6S",
					"user_id": "U03G81FQM98",
					"is_bot": true,
					"is_enterprise_install": false
			}
	],
	"is_ext_shared_channel": false,
	"event_context": "4-eyJldCI6Im1lc3NhZ2UiLCJ0aWQiOiJUMDNDTjVLVEE2UyIsImFpZCI6IkEwM0ZUQzhNWjYzIiwiY2lkIjoiQzAzQ1VRUUJIRUYifQ"
}`

func setSendUrl(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var handleTestCases = []ChannelHandleTestCase{
	{
		Label:      "Receive Hello Msg",
		URL:        receiveURL,
		Headers:    map[string]string{},
		Data:       helloMsg,
		URN:        Sp("slack:C0123ABCDEF"),
		Text:       Sp("Hello World!"),
		Status:     200,
		Response:   "Accepted",
		ExternalID: Sp("Ev0PV52K21"),
	},
	{
		Label:      "Receive image file",
		URL:        receiveURL,
		Headers:    map[string]string{},
		Data:       imageFileMsg,
		Attachment: Sp("https://files.slack.com/files-pri/T03CN5KTA6S-F03GTH43SSF/download/batata.jpg?pub_secret=39fcf577f2"),
		URN:        Sp("slack:C0123ABCDEF"),
		Text:       Sp(""),
		Status:     200,
		Response:   "Accepted",
		ExternalID: Sp("Ev0PV52K21"),
	},
	{
		Label:      "Receive audio file",
		URL:        receiveURL,
		Headers:    map[string]string{},
		Data:       audioFileMsg,
		Attachment: Sp("https://files.slack.com/files-pri/T03CN5KTA6S-F03GWURCZL4/download/here_we_go_again.mp3?pub_secret=471020b300"),
		URN:        Sp("slack:C0123ABCDEF"),
		Text:       Sp(""),
		Status:     200,
		Response:   "Accepted",
		ExternalID: Sp("Ev0PV52K21"),
	},
	{
		Label:      "Receive video file (not allowed)",
		URL:        receiveURL,
		Headers:    map[string]string{},
		Data:       videoFileMsg,
		Attachment: nil,
		URN:        Sp("slack:C0123ABCDEF"),
		Text:       Sp(""),
		Status:     200,
		Response:   "Accepted",
		ExternalID: Sp("Ev0PV52K21"),
	},
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label: "Plain Send",
		Text:  "Simple Message", URN: "slack:C0123ABCDEF",
		Status:         "W",
		ResponseBody:   `{"ok":true,"channel":"C0123ABCDEF"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"C0123ABCDEF","text":"Simple Message"}`,
		SendPrep:       setSendUrl,
	},
	{
		Label: "Unicode Send",
		Text:  "☺", URN: "slack:U0123ABCDEF",
		Status:         "W",
		ResponseBody:   `{"ok":true,"channel":"U0123ABCDEF"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"U0123ABCDEF","text":"☺"}`,
		SendPrep:       setSendUrl,
	},
	{
		Label: "Send Text Auth Error",
		Text:  "Hello", URN: "slack:U0123ABCDEF",
		Status:         "E",
		ResponseBody:   `{"ok":false,"error":"invalid_auth"}`,
		ResponseStatus: 200,
		RequestBody:    `{"channel":"U0123ABCDEF","text":"Hello"}`,
		SendPrep:       setSendUrl,
	},
}

var fileSendTestCases = []ChannelSendTestCase{
	{
		Label: "Send Image",
		Text:  "", URN: "slack:U0123ABCDEF",
		Status:      "W",
		Attachments: []string{"image/jpeg:https://foo.bar/image.png"},
		Responses: map[MockedRequest]MockedResponse{
			{
				Method:       "POST",
				Path:         "/files.upload",
				BodyContains: "image.png",
			}: {
				Status: 200,
				Body:   `{"ok":true,"file":{"id":"F1L3SL4CK1D"}}`,
			},
		},
		SendPrep: setSendUrl,
	},
}

func TestHandler(t *testing.T) {
	slackServiceMock := buildMockSlackService(handleTestCases)
	defer slackServiceMock.Close()

	RunChannelTestCases(t, testChannels, newHandler(), handleTestCases)
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannels[0], newHandler(), defaultSendTestCases, nil)
}

func TestSendFiles(t *testing.T) {
	fileServer := buildMockAttachmentFileServer()
	defer fileServer.Close()
	fileSendTestCases := mockAttachmentURLs(fileServer, fileSendTestCases)

	RunChannelSendTestCases(t, testChannels[0], newHandler(), fileSendTestCases, nil)
}

func TestVerification(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), []ChannelHandleTestCase{
		{Label: "Valid token", URL: receiveURL, Status: 200,
			Data:     `{"token":"one-long-verification-token","challenge":"challenge123","type":"url_verification"}`,
			Headers:  map[string]string{"content-type": "text/plain"},
			Response: "challenge123", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		},
		{Label: "Invalid token", URL: receiveURL, Status: 403,
			Data:    `{"token":"abc321","challenge":"challenge123","type":"url_verification"}`,
			Headers: map[string]string{"content-type": "text/plain"},
		},
	})
}

func buildMockAttachmentFileServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.WriteHeader(200)
		w.Write([]byte("filetype... ...file bytes... ...end"))
	}))
}

func buildMockSlackService(testCases []ChannelHandleTestCase) *httptest.Server {

	files := make(map[string]File)

	for _, tc := range testCases {
		var mp moPayload
		if err := json.Unmarshal([]byte(tc.Data), &mp); err != nil {
			continue
		}

		for _, f := range mp.Event.Files {
			if _, ok := files[f.ID]; ok == false {
				files[f.ID] = f
			}
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		byteBody, err := io.ReadAll(r.Body)
		f, err := jsonparser.GetString(byteBody, "file")
		if err != nil {
			log.Fatal(err)
		}
		defer r.Body.Close()

		file, ok := files[f]

		if file.Mimetype == "video/mp4" {
			w.Write([]byte(`{"ok":"false","error":"public_video_not_allowed"}`))
			return
		}

		if !ok {
			w.Write([]byte(`{"ok": "false", "error": "file not found"}`))
			return
		}
		json.NewEncoder(w).Encode(FileResponse{OK: true, Error: "", File: file})
	}))

	apiURL = server.URL

	return server
}

func mockAttachmentURLs(fileServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase
		for j, attachment := range testCase.Attachments {
			mockedCase.Attachments[j] = strings.Replace(attachment, "https://foo.bar", fileServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}
