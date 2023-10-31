package meta

import (
	"context"
	"fmt"
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

var facebookTestChannels = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FBA", "12345", "", map[string]any{courier.ConfigAuthToken: "a123"}),
}

var facebookIncomingTests = []IncomingTestCase{
	{
		Label:                 "Receive Message FBA",
		URL:                   "/c/fba/receive",
		Data:                  string(test.ReadFile("./testdata/fba/hello_msg.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "Handled",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		ExpectedMsgText:       Sp("Hello World"),
		ExpectedURN:           "facebook:5678",
		ExpectedExternalID:    "external_id",
		ExpectedDate:          time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Invalid Signature",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/hello_msg.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "No Duplicate Receive Message",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/duplicate_msg.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("Hello World"),
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Attachment",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/attachment.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"https://image-url/foo.png"},
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Location",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/location_attachment.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp(""),
		ExpectedAttachments:  []string{"geo:1.200000,-1.300000"},
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Thumbs Up",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/thumbs_up.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgText:      Sp("👍"),
		ExpectedURN:          "facebook:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive OptIn UserRef",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/referral_optin_user_ref.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:ref:optin_user_ref", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "optin_ref"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive OptIn",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/referral_optin.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "optin_ref"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Notification Messages OptIn",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/notification_messages_optin.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeOptIn, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "Bird Facts", "payload": "3456"}},
		},
		ExpectedURNAuthTokens: map[urns.URN]map[string]string{"facebook:5678": {"optin:3456": "12345678901234567890"}},
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Notification Messages OptOut",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/notification_messages_optout.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeOptOut, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "Bird Facts", "payload": "3456"}},
		},
		ExpectedURNAuthTokens: map[urns.URN]map[string]string{"facebook:5678": {}},
		PrepRequest:           addValidSignature,
	},
	{
		Label:                "Receive Get Started",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postback_get_started.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeNewConversation, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "get_started"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Referral Postback",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postback.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Referral",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postback_referral.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type", "ad_id": "ad id"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:                "Receive Referral",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/referral.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"referrer_id":"referral id"`,
		ExpectedEvents: []ExpectedEvent{
			{Type: courier.EventTypeReferral, URN: "facebook:5678", Time: time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC), Extra: map[string]string{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"}},
		},
		PrepRequest: addValidSignature,
	},
	{
		Label:              "Receive Fallback Attachment",
		URL:                "/c/fba/receive",
		Data:               string(test.ReadFile("./testdata/fba/fallback.json")),
		ExpectedRespStatus: 200,
		ExpectedEvents:     []ExpectedEvent{},
		PrepRequest:        addValidSignature,
	},
	{
		Label:                "Receive DLR",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/dlr.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedStatuses:     []ExpectedStatus{{ExternalID: "mid.1458668856218:ed81099e15d3f4f233", Status: courier.MsgStatusDelivered}},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Different Page",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/different_page.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"data":[]`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Echo",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/echo.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring echo`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not Page",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/not_page.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notpage",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Entries",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/no_entries.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no entries found",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Messaging Entries",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/no_messaging_entries.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Unknown Messaging Entry",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/unknown_messaging_entry.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not JSON",
		URL:                  "/c/fba/receive",
		Data:                 "not JSON",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse request JSON",
		NoLogsExpected:       true,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/invalid_urn.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid facebook id",
		PrepRequest:          addValidSignature,
	},
}

func TestFacebookIncoming(t *testing.T) {
	graphURL = createMockGraphAPI().URL

	RunIncomingTestCases(t, facebookTestChannels, newHandler("FBA", "Facebook"), facebookIncomingTests)
}

func TestFacebookDescribeURN(t *testing.T) {
	fbGraph := buildMockFBGraphFBA(facebookIncomingTests)
	defer fbGraph.Close()

	channel := facebookTestChannels[0]
	handler := newHandler("FBA", "Facebook")
	handler.Initialize(test.NewMockServer(courier.NewConfig(), test.NewMockBackend()))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
		{"facebook:ref:1337", map[string]string{}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(courier.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestFacebookVerify(t *testing.T) {
	RunIncomingTestCases(t, facebookTestChannels, newHandler("FBA", "Facebook"), []IncomingTestCase{
		{
			Label:                 "Valid Secret",
			URL:                   "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "yarchallenge",
			NoLogsExpected:        true,
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
		{
			Label:                "Verify No Mode",
			URL:                  "/c/fba/receive",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "unknown request",
			NoLogsExpected:       true,
		},
		{
			Label:                "Verify No Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
			NoLogsExpected:       true,
		},
		{
			Label:                "Invalid Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe&hub.verify_token=blah",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
			NoLogsExpected:       true,
		},
		{
			Label:                "Valid Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "yarchallenge",
			NoLogsExpected:       true,
		},
	})
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.MsgOut) {
	sendURL = s.URL
	graphURL = s.URL
}

var facebookOutgoingTests = []OutgoingTestCase{
	{
		Label:               "Text only chat message",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:12345",
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
		MsgURN:              "facebook:12345",
		MsgOrigin:           courier.MsgOriginBroadcast,
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Text only broadcast with opt-in auth token",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:12345",
		MsgURNAuth:          "345678",
		MsgOrigin:           courier.MsgOriginBroadcast,
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"notification_messages_token":"345678"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Text only flow response",
		MsgText:                 "Simple Message",
		MsgURN:                  "facebook:12345",
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
		Label:                   "Text only flow response using referal URN",
		MsgText:                 "Simple Message",
		MsgURN:                  "facebook:ref:67890",
		MsgOrigin:               courier.MsgOriginFlow,
		MsgResponseToExternalID: "23526",
		MockResponseBody:        `{"message_id": "mid.133", "recipient_id": "12345"}`,
		MockResponseStatus:      200,
		ExpectedRequestBody:     `{"messaging_type":"RESPONSE","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		ExpectedContactURNs:     map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		ExpectedMsgStatus:       "W",
		ExpectedExternalID:      "mid.133",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Quick replies on a broadcast message",
		MsgText:             "Are you happy?",
		MsgURN:              "facebook:12345",
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
		MsgURN:              "facebook:12345",
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
		MsgURN:              "facebook:12345",
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
		MsgURN:              "facebook:12345",
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
		Label:               "Document attachment",
		MsgURN:              "facebook:12345",
		MsgAttachments:      []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Opt-in request",
		MsgURN:              "facebook:12345",
		MsgOptIn:            &courier.OptInReference{ID: 3456, Name: "Joke Of The Day"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"template","payload":{"template_type":"notification_messages","title":"Joke Of The Day","payload":"3456"}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:              "Response doesn't contain message id",
		MsgText:            "ID Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response status code is non-200",
		MsgText:            "Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response is invalid JSON",
		MsgText:            "Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `bad json`,
		MockResponseStatus: 200,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseUnparseable("JSON")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
	{
		Label:              "Response is channel specific error",
		MsgText:            "Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "error": {"message": "The image size is too large.","code": 36000 }}`,
		MockResponseStatus: 400,
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorExternal("36000", "The image size is too large.")},
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

func TestFacebookOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", map[string]any{courier.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTestCases(t, channel, newHandler("FBA", "Facebook"), facebookOutgoingTests, checkRedacted, nil)
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"f39034b29165ec6a5104d9aef27266484ab26c8caa7bca8bcb2dd02e8be61b17",
		},
		{
			"hello world2",
			"60905fdf409d0b4f721e99f6f25b31567a68a6b45e933d814e17a246be4c5a53",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}

func TestFacebookBuildAttachmentRequest(t *testing.T) {
	mb := test.NewMockBackend()
	s := courier.NewServer(courier.NewConfig(), mb)

	handler := &handler{NewBaseHandler(courier.ChannelType("FBA"), "Facebook", DisableUUIDRouting())}
	handler.Initialize(s)
	req, _ := handler.BuildAttachmentRequest(context.Background(), mb, facebookTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)
}

func createMockGraphAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "Bearer a123" && accessToken != "Bearer wac_admin_system_user_token" {
			fmt.Printf("Access token: %s\n", accessToken)
			http.Error(w, "invalid auth token", http.StatusForbidden)
			return
		}

		if strings.HasSuffix(r.URL.Path, "image") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Image"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "audio") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Audio"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "voice") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Voice"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "video") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Video"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "document") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Document"}`))
			return
		}

		// valid token
		w.Write([]byte(`{"url": "https://foo.bar/attachmentURL"}`))
	}))
}

func addValidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha256=%s", string(sig)))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

// mocks the call to the Facebook graph API
func buildMockFBGraphFBA(testCases []IncomingTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", http.StatusForbidden)
		}

		// user has a name
		if strings.HasSuffix(r.URL.Path, "1337") {
			w.Write([]byte(`{ "first_name": "John", "last_name": "Doe"}`))
			return
		}
		// no name
		w.Write([]byte(`{ "first_name": "", "last_name": ""}`))
	}))
	graphURL = server.URL

	return server
}
