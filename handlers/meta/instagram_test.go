package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var instgramTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "12345", "", map[string]any{courier.ConfigAuthToken: "a123"}),
}

var instagramIncomingTests = []IncomingTestCase{
	{
		Label:                 "Receive Message",
		URL:                   "/c/ig/receive",
		Data:                  string(test.ReadFile("./testdata/ig/hello_msg.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "instagram:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Invalid Signature",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/hello_msg.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "No Duplicate Receive Message",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/duplicate_msg.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "instagram:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Attachment",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/attachment.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://image-url/foo.png"},
		ExpectedURN:          "instagram:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Like Heart",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/like_heart.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("❤️"),
		ExpectedURN:          "instagram:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Icebreaker Get Started",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/icebreaker_get_started.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "instagram:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "icebreaker question", "payload": "get_started"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Different Page",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/different_page.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"data":[]`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Echo",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/echo.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring echo`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Entries",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/no_entries.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no entries found",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not Instagram",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/not_instagram.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notinstagram",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Messaging Entries",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/no_messaging_entries.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Unknown Messaging Entry",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/unknown_messaging_entry.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not JSON",
		URL:                  "/c/ig/receive",
		Data:                 "not JSON",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse request JSON",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/invalid_urn.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid instagram id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Story Mention",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/story_mention.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring story_mention`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Message unsent",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/unsent_msg.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `msg deleted`,
		PrepRequest:          addValidSignature,
	},
}

func TestInstagramIncoming(t *testing.T) {
	graphURL = createMockGraphAPI().URL

	RunIncomingTestCases(t, instgramTestChannels, newHandler("IG", "Instagram"), instagramIncomingTests)
}

var instagramOutgoingTests = []OutgoingTestCase{
	{
		Label:               "Text only chat message",
		MsgText:             "Simple Message",
		MsgURN:              "instagram:12345",
		MsgOrigin:           courier.MsgOriginChat,
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Text only broadcast message",
		MsgText:             "Simple Message",
		MsgURN:              "instagram:12345",
		MsgOrigin:           courier.MsgOriginBroadcast,
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Text only flow response",
		MsgText:                 "Simple Message",
		MsgURN:                  "instagram:12345",
		MsgOrigin:               courier.MsgOriginFlow,
		MsgResponseToExternalID: "23526",
		MockResponseBody:        `{"message_id": "mid.133"}`,
		MockResponseStatus:      200,
		ExpectedRequestBody:     `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:       "W",
		ExpectedExternalID:      "mid.133",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Quick replies on a broadcast message",
		MsgText:             "Are you happy?",
		MsgURN:              "instagram:12345",
		MsgOrigin:           courier.MsgOriginBroadcast,
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Message that exceeds max text length",
		MsgText:             "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		MsgURN:              "instagram:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MsgTopic:            "account",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"ACCOUNT_UPDATE","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Image attachment",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Text, image attachment, quick replies and explicit message topic",
		MsgText:             "This is some text.",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgQuickReplies:     []string{"Yes", "No"},
		MsgTopic:            "event",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"CONFIRMED_EVENT_UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Explicit human agent tag",
		MsgText:             "Simple Message",
		MsgURN:              "instagram:12345",
		MsgTopic:            "agent",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Document attachment",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Response doesn't contain message id",
		MsgText:            "ID Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response status code is non-200",
		MsgText:            "Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response is invalid JSON",
		MsgText:            "Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `bad json`,
		MockResponseStatus: 200,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response is channel specific error",
		MsgText:            "Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `{ "error": {"message": "The image size is too large.","code": 36000 }}`,
		MockResponseStatus: 400,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("36000", "The image size is too large.")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestInstagramOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "12345", "", map[string]any{courier.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTestCases(t, channel, newHandler("IG", "Instagram"), instagramOutgoingTests, checkRedacted, nil)
}

func TestInstgramVerify(t *testing.T) {
	RunIncomingTestCases(t, instgramTestChannels, newHandler("IG", "Instagram"), []IncomingTestCase{
		{
			Label:                 "Valid Secret",
			URL:                   "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "yarchallenge",
			NoLogsExpected:        true,
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
		{
			Label:                "Verify No Mode",
			URL:                  "/c/ig/receive",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "unknown request",
			NoLogsExpected:       true,
		},
		{
			Label:                "Verify No Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
			NoLogsExpected:       true,
		},
		{
			Label:                "Invalid Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe&hub.verify_token=blah",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
			NoLogsExpected:       true,
		},
		{
			Label:                "Valid Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "yarchallenge",
			NoLogsExpected:       true,
		},
	})
}

func TestInstagramDescribeURN(t *testing.T) {
	fbGraph := buildMockFBGraphIG(instagramIncomingTests)
	defer fbGraph.Close()

	channel := instgramTestChannels[0]
	handler := newHandler("IG", "Instagram")
	handler.Initialize(test.NewMockServer(courier.NewConfig(), test.NewMockBackend()))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"instagram:1337", map[string]string{"name": "John Doe"}},
		{"instagram:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(courier.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestInstagramBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()
	s := courier.NewServer(courier.NewConfig(), mb)

	handler := &handler{NewBaseHandler(courier.ChannelType("IG"), "Instagram", DisableUUIDRouting())}
	handler.Initialize(s)
	req, _ := handler.BuildAttachmentRequest(context.Background(), mb, facebookTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)
}

// mocks the call to the Facebook graph API
func buildMockFBGraphIG(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", http.StatusForbidden)
		}

		// user has a name
		if strings.HasSuffix(r.URL.Path, "1337") {
			w.Write([]byte(`{ "name": "John Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "name": ""}`))
	}))
	graphURL = server.URL

	return server
}
