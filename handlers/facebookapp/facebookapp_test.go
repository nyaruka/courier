package facebookapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannelsFBA = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testChannelsIG = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testChannelsWAC = []courier.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testCasesFBA = []ChannelHandleTestCase{
	{
		Label:                 "Receive Message FBA",
		URL:                   "/c/fba/receive",
		Data:                  string(test.ReadFile("./testdata/fba/helloMsgFBA.json")),
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
		Data:                 string(test.ReadFile("./testdata/fba/helloMsgFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "No Duplicate Receive Message",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/duplicateMsgFBA.json")),
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
		Data:                 string(test.ReadFile("./testdata/fba/attachmentFBA.json")),
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
		Data:                 string(test.ReadFile("./testdata/fba/locationAttachment.json")),
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
		Data:                 string(test.ReadFile("./testdata/fba/thumbsUp.json")),
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
		Data:                 string(test.ReadFile("./testdata/fba/optInUserRef.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "facebook:ref:optin_user_ref",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.Referral,
		ExpectedEventExtra:   map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive OptIn",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/optIn.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "facebook:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.Referral,
		ExpectedEventExtra:   map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Get Started",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postbackGetStarted.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "facebook:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.NewConversation,
		ExpectedEventExtra:   map[string]interface{}{"title": "postback title", "payload": "get_started"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Referral Postback",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postback.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "facebook:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.Referral,
		ExpectedEventExtra:   map[string]interface{}{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Referral",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/postbackReferral.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "facebook:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.Referral,
		ExpectedEventExtra:   map[string]interface{}{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type", "ad_id": "ad id"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Referral",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/referral.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"referrer_id":"referral id"`,
		ExpectedURN:          "facebook:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.Referral,
		ExpectedEventExtra:   map[string]interface{}{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive DLR",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/dlr.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedMsgStatus:    courier.MsgDelivered,
		ExpectedExternalID:   "mid.1458668856218:ed81099e15d3f4f233",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Different Page",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/differentPageFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"data":[]`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Echo",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/echoFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring echo`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not Page",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/notPage.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notpage",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Entries",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/noEntriesFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no entries found",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Messaging Entries",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/noMessagingEntriesFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Unknown Messaging Entry",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/unknownMessagingEntryFBA.json")),
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
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/fba/receive",
		Data:                 string(test.ReadFile("./testdata/fba/invalidURNFBA.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid facebook id",
		PrepRequest:          addValidSignature,
	},
}

var testCasesIG = []ChannelHandleTestCase{
	{
		Label:                 "Receive Message",
		URL:                   "/c/ig/receive",
		Data:                  string(test.ReadFile("./testdata/ig/helloMsgIG.json")),
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
		Data:                 string(test.ReadFile("./testdata/ig/helloMsgIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid request signature",
		PrepRequest:          addInvalidSignature,
	},
	{
		Label:                "No Duplicate Receive Message",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/duplicateMsgIG.json")),
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
		Data:                 string(test.ReadFile("./testdata/ig/attachmentIG.json")),
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
		ExpectedMsgText:      Sp(""),
		ExpectedURN:          "instagram:5678",
		ExpectedExternalID:   "external_id",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Icebreaker Get Started",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/icebreakerGetStarted.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		ExpectedURN:          "instagram:5678",
		ExpectedDate:         time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC),
		ExpectedEvent:        courier.NewConversation,
		ExpectedEventExtra:   map[string]interface{}{"title": "icebreaker question", "payload": "get_started"},
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Different Page",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/differentPageIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"data":[]`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Echo",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/echoIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring echo`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Entries",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/noEntriesIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "no entries found",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Not Instagram",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/notInstagram.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notinstagram",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "No Messaging Entries",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/noMessagingEntriesIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "Handled",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Unknown Messaging Entry",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/unknownMessagingEntryIG.json")),
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
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Invalid URN",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/invalidURNIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid instagram id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Story Mention",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/storyMentionIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `ignoring story_mention`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Message unsent",
		URL:                  "/c/ig/receive",
		Data:                 string(test.ReadFile("./testdata/ig/unsentMsgIG.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `msg deleted`,
		PrepRequest:          addValidSignature,
	},
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
func buildMockFBGraphFBA(testCases []ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", 403)
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

// mocks the call to the Facebook graph API
func buildMockFBGraphIG(testCases []ChannelHandleTestCase) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", 403)
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

func TestDescribeURNForFBA(t *testing.T) {
	fbGraph := buildMockFBGraphFBA(testCasesFBA)
	defer fbGraph.Close()

	channel := testChannelsFBA[0]
	handler := newHandler("FBA", "Facebook", false)
	handler.Initialize(newServer(nil))
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

func TestDescribeURNForIG(t *testing.T) {
	fbGraph := buildMockFBGraphIG(testCasesIG)
	defer fbGraph.Close()

	channel := testChannelsIG[0]
	handler := newHandler("IG", "Instagram", false)
	handler.Initialize(newServer(nil))
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

func TestDescribeURNForWAC(t *testing.T) {
	channel := testChannelsWAC[0]
	handler := newHandler("WAC", "Cloud API WhatsApp", false)
	handler.Initialize(newServer(nil))
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"whatsapp:1337", map[string]string{}},
		{"whatsapp:4567", map[string]string{}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(courier.URNDescriber).DescribeURN(context.Background(), testChannelsWAC[0], tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

var wacReceiveURL = "/c/wac/receive"

var testCasesWAC = []ChannelHandleTestCase{
	{
		Label:                 "Receive Message WAC",
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/helloWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/duplicateWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/voiceWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/buttonWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/documentWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/imageWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/videoWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/audioWAC.json")),
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
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/locationWAC.json")),
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
		URL:                  wacReceiveURL,
		Data:                 "not json",
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "unable to parse",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid JSON",
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidFrom.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid whatsapp id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid JSON",
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidTimestamp.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: "invalid timestamp",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                 "Receive Message WAC invalid signature",
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/helloWAC.json")),
		ExpectedRespStatus:    200,
		ExpectedBodyContains:  "invalid request signature",
		NoQueueErrorCheck:     true,
		NoInvalidChannelCheck: true,
		PrepRequest:           addInvalidSignature,
	},
	{
		Label:                "Receive Valid Status",
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/validStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"type":"status"`,
		ExpectedMsgStatus:    "S",
		ExpectedExternalID:   "external_id",
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Invalid Status",
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/invalidStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"unknown status: in_orbit"`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                "Receive Ignore Status",
		URL:                  wacReceiveURL,
		Data:                 string(test.ReadFile("./testdata/wac/ignoreStatusWAC.json")),
		ExpectedRespStatus:   200,
		ExpectedBodyContains: `"ignoring status: deleted"`,
		PrepRequest:          addValidSignature,
	},
	{
		Label:                 "Receive Valid Interactive Button Reply Message",
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/buttonReplyWAC.json")),
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
		URL:                   wacReceiveURL,
		Data:                  string(test.ReadFile("./testdata/wac/listReplyWAC.json")),
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

func TestHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "Bearer a123" && accessToken != "Bearer wac_admin_system_user_token" {
			fmt.Printf("Access token: %s\n", accessToken)
			http.Error(w, "invalid auth token", 403)
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
	graphURL = server.URL

	RunChannelTestCases(t, testChannelsWAC, newHandler("WAC", "Cloud API WhatsApp", false), testCasesWAC)
	RunChannelTestCases(t, testChannelsFBA, newHandler("FBA", "Facebook", false), testCasesFBA)
	RunChannelTestCases(t, testChannelsIG, newHandler("IG", "Instagram", false), testCasesIG)
}

func BenchmarkHandler(b *testing.B) {
	fbService := buildMockFBGraphFBA(testCasesFBA)

	RunChannelBenchmarks(b, testChannelsFBA, newHandler("FBA", "Facebook", false), testCasesFBA)
	fbService.Close()

	fbServiceIG := buildMockFBGraphIG(testCasesIG)

	RunChannelBenchmarks(b, testChannelsIG, newHandler("IG", "Instagram", false), testCasesIG)
	fbServiceIG.Close()
}

func TestVerify(t *testing.T) {
	RunChannelTestCases(t, testChannelsFBA, newHandler("FBA", "Facebook", false), []ChannelHandleTestCase{
		{
			Label:                 "Valid Secret",
			URL:                   "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "yarchallenge",
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
		{
			Label:                "Verify No Mode",
			URL:                  "/c/fba/receive",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "unknown request",
		},
		{
			Label:                "Verify No Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Invalid Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe&hub.verify_token=blah",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Valid Secret",
			URL:                  "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "yarchallenge",
		},
	})

	RunChannelTestCases(t, testChannelsIG, newHandler("IG", "Instagram", false), []ChannelHandleTestCase{
		{
			Label:                 "Valid Secret",
			URL:                   "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:    200,
			ExpectedBodyContains:  "yarchallenge",
			NoQueueErrorCheck:     true,
			NoInvalidChannelCheck: true,
		},
		{
			Label:                "Verify No Mode",
			URL:                  "/c/ig/receive",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "unknown request",
		},
		{
			Label:                "Verify No Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Invalid Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe&hub.verify_token=blah",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "token does not match secret",
		},
		{
			Label:                "Valid Secret",
			URL:                  "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge",
			ExpectedRespStatus:   200,
			ExpectedBodyContains: "yarchallenge",
		},
	})
}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
	graphURL = s.URL
}

var SendTestCasesFBA = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:12345",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Plain Response",
		MsgText:                 "Simple Message",
		MsgURN:                  "facebook:12345",
		MsgResponseToExternalID: "23526",
		MockResponseBody:        `{"message_id": "mid.133"}`,
		MockResponseStatus:      200,
		ExpectedRequestBody:     `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:       "W",
		ExpectedExternalID:      "mid.133",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Plain Send using ref URN",
		MsgText:             "Simple Message",
		MsgURN:              "facebook:ref:67890",
		MockResponseBody:    `{"message_id": "mid.133", "recipient_id": "12345"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		ExpectedContactURNs: map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "facebook:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Message",
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
		Label:               "Send Photo",
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
		Label:               "Send caption and photo with Quick Reply",
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
		Label:               "Send Document",
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
		Label:              "ID Error",
		MsgText:            "ID Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "facebook:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

var SendTestCasesIG = []ChannelSendTestCase{
	{
		Label:               "Plain Send",
		MsgText:             "Simple Message",
		MsgURN:              "instagram:12345",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:                   "Plain Response",
		MsgText:                 "Simple Message",
		MsgURN:                  "instagram:12345",
		MsgResponseToExternalID: "23526",
		MockResponseBody:        `{"message_id": "mid.133"}`,
		MockResponseStatus:      200,
		ExpectedRequestBody:     `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:       "W",
		ExpectedExternalID:      "mid.133",
		SendPrep:                setSendURL,
	},
	{
		Label:               "Quick Reply",
		MsgText:             "Are you happy?",
		MsgURN:              "instagram:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Long Message",
		MsgText:             "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		MsgURN:              "instagram:12345",
		MsgQuickReplies:     []string{"Yes", "No"},
		MsgTopic:            "agent",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL,
	},
	{
		Label:               "Send Photo",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL},
	{
		Label:               "Send caption and photo with Quick Reply",
		MsgText:             "This is some text.",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"image/jpeg:https://foo.bar/image.jpg"},
		MsgQuickReplies:     []string{"Yes", "No"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL},
	{
		Label:               "Tag Human Agent",
		MsgText:             "Simple Message",
		MsgURN:              "instagram:12345",
		MsgTopic:            "agent",
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL},
	{
		Label:               "Send Document",
		MsgURN:              "instagram:12345",
		MsgAttachments:      []string{"application/pdf:https://foo.bar/document.pdf"},
		MockResponseBody:    `{"message_id": "mid.133"}`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "mid.133",
		SendPrep:            setSendURL},
	{
		Label:              "ID Error",
		MsgText:            "ID Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 200,
		ExpectedMsgStatus:  "E",
		ExpectedErrors:     []*courier.ChannelError{courier.ErrorResponseValueMissing("message_id")},
		SendPrep:           setSendURL,
	},
	{
		Label:              "Error",
		MsgText:            "Error",
		MsgURN:             "instagram:12345",
		MockResponseBody:   `{ "is_error": true }`,
		MockResponseStatus: 403,
		ExpectedMsgStatus:  "E",
		SendPrep:           setSendURL,
	},
}

var SendTestCasesWAC = []ChannelSendTestCase{
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
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf","caption":"document caption"}}`,
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
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		SendPrep:            setSendURL,
	},
	{
		Label:               "Template Country Language",
		MsgText:             "templated message",
		MsgURN:              "whatsapp:250788123123",
		MsgMetadata:         json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "country": "US", "variables": ["Chef", "tomorrow"]}}`),
		MockResponseBody:    `{ "messages": [{"id": "157b5e14568e8"}] }`,
		MockResponseStatus:  200,
		ExpectedRequestBody: `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		ExpectedMsgStatus:   "W",
		ExpectedExternalID:  "157b5e14568e8",
		SendPrep:            setSendURL,
	},
	{
		Label:          "Template Invalid Language",
		MsgText:        "templated message",
		MsgURN:         "whatsapp:250788123123",
		MsgMetadata:    json.RawMessage(`{"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}}`),
		ExpectedErrors: []*courier.ChannelError{courier.NewChannelError(`unable to decode template: {"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}} for channel: 8eb23e93-5ecb-45ba-b726-3b064e0c56ab: unable to find mapping for language: bnt`, "")},
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
		Label:           "Interactive Button Message Send with attachment",
		MsgText:         "Interactive Button Msg",
		MsgURN:          "whatsapp:250788123123",
		MsgQuickReplies: []string{"BUTTON1"},
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
				Body:   `{"messaging_product":"whatsapp","recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
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
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	var ChannelFBA = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	var ChannelIG = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	var ChannelWAC = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunChannelSendTestCases(t, ChannelFBA, newHandler("FBA", "Facebook", false), SendTestCasesFBA, checkRedacted, nil)
	RunChannelSendTestCases(t, ChannelIG, newHandler("IG", "Instagram", false), SendTestCasesIG, checkRedacted, nil)
	RunChannelSendTestCases(t, ChannelWAC, newHandler("WAC", "Cloud API WhatsApp", false), SendTestCasesWAC, checkRedacted, nil)
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

func newServer(backend courier.Backend) courier.Server {
	config := courier.NewConfig()
	config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"
	return courier.NewServer(config, backend)
}

func TestBuildMediaRequest(t *testing.T) {
	mb := test.NewMockBackend()
	s := newServer(mb)
	wacHandler := &handler{NewBaseHandlerWithParams(courier.ChannelType("WAC"), "WhatsApp Cloud", false, nil)}
	wacHandler.Initialize(s)
	req, _ := wacHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannelsWAC[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer wac_admin_system_user_token", req.Header.Get("Authorization"))

	fbaHandler := &handler{NewBaseHandlerWithParams(courier.ChannelType("FBA"), "Facebook", false, nil)}
	fbaHandler.Initialize(s)
	req, _ = fbaHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannelsFBA[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)

	igHandler := &handler{NewBaseHandlerWithParams(courier.ChannelType("IG"), "Instagram", false, nil)}
	igHandler.Initialize(s)
	req, _ = igHandler.BuildDownloadMediaRequest(context.Background(), mb, testChannelsFBA[0], "https://example.org/v1/media/41")
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)
}
