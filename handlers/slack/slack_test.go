package slack

import (
	"context"
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
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

const (
	channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	receiveURL  = "/c/sl/" + channelUUID + "/receive/"
)

var testChannels = []courier.Channel{
	test.NewMockChannel(channelUUID, "SL", "2022", "US", map[string]interface{}{"bot_token": "xoxb-abc123", "verification_token": "one-long-verification-token"}),
}

const helloMsg = `{
	"token": "one-long-verification-token",
	"team_id": "T061EG9R6",
	"api_app_id": "A0PNCHHK2",
	"event": {
			"type": "message",
			"channel": "U0123ABCDEF",
			"user": "U0123ABCDEF",
			"text": "Hello World!",
			"ts": "1355517523.000005",
			"event_ts": "1355517523.000005",
			"channel_type": "im"
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
							"mimetype": "image/jpeg",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GTH43SSF/download/batata.jpg",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GTH43SSF-39fcf577f2"
					}
			],
			"user": "U0123ABCDEF",
			"channel": "U0123ABCDEF",
			"channel_type": "im"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653417052
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
							"mimetype": "audio/mpeg",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GWURCZL4/download/here_we_go_again.mp3",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GWURCZL4-471020b300"
					}
			],
			"user": "U0123ABCDEF",
			"channel": "U0123ABCDEF",
			"channel_type": "im"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653428835
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
							"mimetype": "video/mp4",
							"url_private_download": "https://files.slack.com/files-pri/T03CN5KTA6S-F03GDSSMC79/download/walk_cycle_animation_sample.mp4",
							"permalink": "https://teste-apigrupo.slack.com/files/U0123ABCDEF/F03GDSSMC79/walk_cycle_animation_sample.mp4",
							"permalink_public": "https://slack-files.com/T03CN5KTA6S-F03GDSSMC79-805aa1d85f"
					}
			],
			"user": "U0123ABCDEF",
			"channel": "U0123ABCDEF",
			"channel_type": "im"
	},
	"type": "event_callback",
	"event_id": "Ev0PV52K21",
	"event_time": 1653427243
}`

func setSendUrl(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	apiURL = s.URL
}

var handleTestCases = []ChannelHandleTestCase{
	{
		Label:              "Receive Hello Msg",
		URL:                receiveURL,
		Headers:            map[string]string{},
		Data:               helloMsg,
		ExpectedURN:        Sp("slack:U0123ABCDEF"),
		ExpectedMsgText:    Sp("Hello World!"),
		ExpectedStatus:     200,
		ExpectedResponse:   "Accepted",
		ExpectedExternalID: Sp("Ev0PV52K21"),
	},
	{
		Label:               "Receive image file",
		URL:                 receiveURL,
		Headers:             map[string]string{},
		Data:                imageFileMsg,
		ExpectedAttachments: []string{"https://files.slack.com/files-pri/T03CN5KTA6S-F03GTH43SSF/download/batata.jpg?pub_secret=39fcf577f2"},
		ExpectedURN:         Sp("slack:U0123ABCDEF"),
		ExpectedMsgText:     Sp(""),
		ExpectedStatus:      200,
		ExpectedResponse:    "Accepted",
		ExpectedExternalID:  Sp("Ev0PV52K21"),
	},
	{
		Label:               "Receive audio file",
		URL:                 receiveURL,
		Headers:             map[string]string{},
		Data:                audioFileMsg,
		ExpectedAttachments: []string{"https://files.slack.com/files-pri/T03CN5KTA6S-F03GWURCZL4/download/here_we_go_again.mp3?pub_secret=471020b300"},
		ExpectedURN:         Sp("slack:U0123ABCDEF"),
		ExpectedMsgText:     Sp(""),
		ExpectedStatus:      200,
		ExpectedResponse:    "Accepted",
		ExpectedExternalID:  Sp("Ev0PV52K21"),
	},
	{
		Label:              "Receive video file (not allowed)",
		URL:                receiveURL,
		Headers:            map[string]string{},
		Data:               videoFileMsg,
		ExpectedURN:        Sp("slack:U0123ABCDEF"),
		ExpectedMsgText:    Sp(""),
		ExpectedStatus:     200,
		ExpectedResponse:   "Accepted",
		ExpectedExternalID: Sp("Ev0PV52K21"),
	},
}

var defaultSendTestCases = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "slack:U0123ABCDEF",
		MockResponseBody:    `{"ok":true,"channel":"U0123ABCDEF"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"channel":"U0123ABCDEF","text":"Simple Message"}`,
		ExpectedStatus:      "W",
		SendPrep:            setSendUrl,
	},
	{
		Label:               "Unicode Send",
		MsgText:             "☺",
		MsgURN:              "slack:U0123ABCDEF",
		MockResponseBody:    `{"ok":true,"channel":"U0123ABCDEF"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"channel":"U0123ABCDEF","text":"☺"}`,
		ExpectedStatus:      "W",
		SendPrep:            setSendUrl,
	},
	{
		Label:               "Send Text Auth Error",
		MsgText:             "Hello",
		MsgURN:              "slack:U0123ABCDEF",
		MockResponseBody:    `{"ok":false,"error":"invalid_auth"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"channel":"U0123ABCDEF","text":"Hello"}`,
		ExpectedStatus:      "E",
		ExpectedErrors:      []string{"invalid_auth"},
		SendPrep:            setSendUrl,
	},
}

var fileSendTestCases = []ChannelSendTestCase{
	{
		Label:          "Send Image",
		MsgText:        "",
		MsgURN:         "slack:U0123ABCDEF",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.png"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/files.upload",
				BodyContains: "image.png",
			}: httpx.NewMockResponse(200, nil, []byte(`{"ok":true,"file":{"id":"F1L3SL4CK1D"}}`)),
		},
		ExpectedStatus: "W",
		SendPrep:       setSendUrl,
	},
	{
		Label:          "Send Image",
		MsgText:        "",
		MsgURN:         "slack:U0123ABCDEF",
		MsgAttachments: []string{"image/jpeg:https://foo.bar/image.png"},
		MockResponses: map[MockedRequest]*httpx.MockResponse{
			{
				Method:       "POST",
				Path:         "/files.upload",
				BodyContains: "image.png",
			}: httpx.NewMockResponse(200, nil, []byte(`{"ok":true,"file":{"id":"F1L3SL4CK1D"}}`)),
		},
		ExpectedStatus: "W",
		SendPrep:       setSendUrl,
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
		{Label: "Valid token", URL: receiveURL, ExpectedStatus: 200,
			Data:             `{"token":"one-long-verification-token","challenge":"challenge123","type":"url_verification"}`,
			Headers:          map[string]string{"content-type": "text/plain"},
			ExpectedResponse: "challenge123", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		},
		{Label: "Invalid token", URL: receiveURL, ExpectedStatus: 403,
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
		switch r.URL.Path {

		case "/users.info":

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"user":{"real_name":"dummy user"}}`))

		case "/files.sharedPublicURL":

			byteBody, err := io.ReadAll(r.Body)
			if err != nil {
				log.Fatal(err)
			}
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
		}
	}))

	apiURL = server.URL

	return server
}

func mockAttachmentURLs(fileServer *httptest.Server, testCases []ChannelSendTestCase) []ChannelSendTestCase {
	casesWithMockedUrls := make([]ChannelSendTestCase, len(testCases))

	for i, testCase := range testCases {
		mockedCase := testCase
		for j, attachment := range testCase.MsgAttachments {
			mockedCase.MsgAttachments[j] = strings.Replace(attachment, "https://foo.bar", fileServer.URL, 1)
		}
		casesWithMockedUrls[i] = mockedCase
	}
	return casesWithMockedUrls
}

func TestDescribeURN(t *testing.T) {
	server := buildMockSlackService([]ChannelHandleTestCase{})
	defer server.Close()

	handler := newHandler().(courier.URNDescriber)
	logger := courier.NewChannelLogger(testChannels[0])
	urn, _ := urns.NewURNFromParts(urns.SlackScheme, "U012345", "", "")

	data := map[string]string{"name": "dummy user"}

	describe, err := handler.DescribeURN(context.Background(), testChannels[0], urn, logger)
	assert.Nil(t, err)
	assert.Equal(t, data, describe)
}
