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
	"github.com/nyaruka/courier/handlers"
	. "github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannelsFBA = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testChannelsIG = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testChannelsWAC = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "WAC", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var testCasesFBA = []ChannelHandleTestCase{
	{Label: "Receive Message FBA", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/helloMsgFBA.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Invalid Signature", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/helloMsgFBA.json")), Status: 400, Response: "invalid request signature", PrepRequest: addInvalidSignature},

	{Label: "No Duplicate Receive Message", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/duplicateMsgFBA.json")), Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Attachment", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/attachmentFBA.json")), Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Location", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/locationAttachment.json")), Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"geo:1.200000,-1.300000"}, URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Thumbs Up", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/thumbsUp.json")), Status: 200, Response: "Handled",
		Text: Sp("👍"), URN: Sp("facebook:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive OptIn UserRef", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/optInUserRef.json")), Status: 200, Response: "Handled",
		URN: Sp("facebook:ref:optin_user_ref"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest: addValidSignature},
	{Label: "Receive OptIn", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/optIn.json")), Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		ChannelEvent: Sp(courier.Referral), ChannelEventExtra: map[string]interface{}{"referrer_id": "optin_ref"},
		PrepRequest: addValidSignature},

	{Label: "Receive Get Started", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/postbackGetStarted.json")), Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started"},
		PrepRequest:       addValidSignature},
	{Label: "Receive Referral Postback", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/postback.json")), Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "postback payload", "referrer_id": "postback ref", "source": "postback source", "type": "postback type"},
		PrepRequest:       addValidSignature},
	{Label: "Receive Referral", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/postbackReferral.json")), Status: 200, Response: "Handled",
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"title": "postback title", "payload": "get_started", "referrer_id": "postback ref", "source": "postback source", "type": "postback type", "ad_id": "ad id"},
		PrepRequest:       addValidSignature},

	{Label: "Receive Referral", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/referral.json")), Status: 200, Response: `"referrer_id":"referral id"`,
		URN: Sp("facebook:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.Referral),
		ChannelEventExtra: map[string]interface{}{"referrer_id": "referral id", "source": "referral source", "type": "referral type", "ad_id": "ad id"},
		PrepRequest:       addValidSignature},

	{Label: "Receive DLR", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/dlr.json")), Status: 200, Response: "Handled",
		Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), MsgStatus: Sp(courier.MsgDelivered), ExternalID: Sp("mid.1458668856218:ed81099e15d3f4f233"),
		PrepRequest: addValidSignature},

	{Label: "Different Page", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/differentPageFBA.json")), Status: 200, Response: `"data":[]`, PrepRequest: addValidSignature},
	{Label: "Echo", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/echoFBA.json")), Status: 200, Response: `ignoring echo`, PrepRequest: addValidSignature},
	{Label: "Not Page", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/notPage.json")), Status: 400, Response: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notpage", PrepRequest: addValidSignature},
	{Label: "No Entries", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/noEntriesFBA.json")), Status: 400, Response: "no entries found", PrepRequest: addValidSignature},
	{Label: "No Messaging Entries", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/noMessagingEntriesFBA.json")), Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Unknown Messaging Entry", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/unknownMessagingEntryFBA.json")), Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Not JSON", URL: "/c/fba/receive", Data: "not JSON", Status: 400, Response: "Error", PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: "/c/fba/receive", Data: string(courier.ReadFile("./testdata/fba/invalidURNFBA.json")), Status: 400, Response: "invalid facebook id", PrepRequest: addValidSignature},
}
var testCasesIG = []ChannelHandleTestCase{
	{Label: "Receive Message", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/helloMsgIG.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Invalid Signature", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/helloMsgIG.json")), Status: 400, Response: "invalid request signature", PrepRequest: addInvalidSignature},

	{Label: "No Duplicate Receive Message", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/duplicateMsgIG.json")), Status: 200, Response: "Handled",
		Text: Sp("Hello World"), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Attachment", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/attachmentIG.json")), Status: 200, Response: "Handled",
		Text: Sp(""), Attachments: []string{"https://image-url/foo.png"}, URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Like Heart", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/like_heart.json")), Status: 200, Response: "Handled",
		Text: Sp(""), URN: Sp("instagram:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Icebreaker Get Started", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/icebreakerGetStarted.json")), Status: 200, Response: "Handled",
		URN: Sp("instagram:5678"), Date: Tp(time.Date(2016, 4, 7, 1, 11, 27, 970000000, time.UTC)), ChannelEvent: Sp(courier.NewConversation),
		ChannelEventExtra: map[string]interface{}{"title": "icebreaker question", "payload": "get_started"},
		PrepRequest:       addValidSignature},
	{Label: "Different Page", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/differentPageIG.json")), Status: 200, Response: `"data":[]`, PrepRequest: addValidSignature},
	{Label: "Echo", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/echoIG.json")), Status: 200, Response: `ignoring echo`, PrepRequest: addValidSignature},
	{Label: "No Entries", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/noEntriesIG.json")), Status: 400, Response: "no entries found", PrepRequest: addValidSignature},
	{Label: "Not Instagram", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/notInstagram.json")), Status: 400, Response: "object expected 'page', 'instagram' or 'whatsapp_business_account', found notinstagram", PrepRequest: addValidSignature},
	{Label: "No Messaging Entries", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/noMessagingEntriesIG.json")), Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Unknown Messaging Entry", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/unknownMessagingEntryIG.json")), Status: 200, Response: "Handled", PrepRequest: addValidSignature},
	{Label: "Not JSON", URL: "/c/ig/receive", Data: "not JSON", Status: 400, Response: "Error", PrepRequest: addValidSignature},
	{Label: "Invalid URN", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/invalidURNIG.json")), Status: 400, Response: "invalid instagram id", PrepRequest: addValidSignature},
	{Label: "Story Mention", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/storyMentionIG.json")), Status: 200, Response: `ignoring story_mention`, PrepRequest: addValidSignature},
	{Label: "Message unsent", URL: "/c/ig/receive", Data: string(courier.ReadFile("./testdata/ig/unsentMsgIG.json")), Status: 200, Response: `msg deleted`, PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, 100000)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha1=%s", string(sig)))
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

func TestDescribeFBA(t *testing.T) {
	fbGraph := buildMockFBGraphFBA(testCasesFBA)
	defer fbGraph.Close()

	handler := newHandler("FBA", "Facebook", false).(courier.URNDescriber)
	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
		{"facebook:ref:1337", map[string]string{}}}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannelsFBA[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}

func TestDescribeIG(t *testing.T) {
	fbGraph := buildMockFBGraphIG(testCasesIG)
	defer fbGraph.Close()

	handler := newHandler("IG", "Instagram", false).(courier.URNDescriber)
	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{{"instagram:1337", map[string]string{"name": "John Doe"}},
		{"instagram:4567", map[string]string{"name": ""}}}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannelsIG[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}

func TestDescribeWAC(t *testing.T) {
	handler := newHandler("WAC", "Cloud API WhatsApp", false).(courier.URNDescriber)

	tcs := []struct {
		urn      urns.URN
		metadata map[string]string
	}{{"whatsapp:1337", map[string]string{}},
		{"whatsapp:4567", map[string]string{}}}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannelsWAC[0], tc.urn)
		assert.Equal(t, metadata, tc.metadata)
	}
}

var wacReceiveURL = "/c/wac/receive"

var testCasesWAC = []ChannelHandleTestCase{
	{Label: "Receive Message WAC", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/helloWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Duplicate Valid Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/duplicateWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Voice Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/voiceWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp(""), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Voice"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Button Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/buttonWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("No"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Document Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/documentWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("80skaraokesonglistartist"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Document"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Image Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/imageWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Image"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Video Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/videoWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Video"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Audio Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/audioWAC.json")), Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Audio"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Location Message", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/locationWAC.json")), Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("geo:0.000000,1.000000"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Invalid JSON", URL: wacReceiveURL, Data: "not json", Status: 400, Response: "unable to parse", PrepRequest: addValidSignature},
	{Label: "Receive Invalid JSON", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/invalidFrom.json")), Status: 400, Response: "invalid whatsapp id", PrepRequest: addValidSignature},
	{Label: "Receive Invalid JSON", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/invalidTimestamp.json")), Status: 400, Response: "invalid timestamp", PrepRequest: addValidSignature},

	{Label: "Receive Valid Status", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/validStatusWAC.json")), Status: 200, Response: `"type":"status"`,
		MsgStatus: Sp("S"), ExternalID: Sp("external_id"), PrepRequest: addValidSignature},
	{Label: "Receive Invalid Status", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/invalidStatusWAC.json")), Status: 400, Response: `"unknown status: in_orbit"`, PrepRequest: addValidSignature},
	{Label: "Receive Ignore Status", URL: wacReceiveURL, Data: string(courier.ReadFile("./testdata/wac/ignoreStatusWAC.json")), Status: 200, Response: `"ignoring status: deleted"`, PrepRequest: addValidSignature},
}

func TestHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "Bearer a123" {
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
		{Label: "Valid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200,
			Response: "yarchallenge", NoQueueErrorCheck: true, NoInvalidChannelCheck: true},
		{Label: "Verify No Mode", URL: "/c/fba/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/fba/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/fba/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

	RunChannelTestCases(t, testChannelsIG, newHandler("IG", "Instagram", false), []ChannelHandleTestCase{
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200,
			Response: "yarchallenge", NoQueueErrorCheck: true, NoInvalidChannelCheck: true},
		{Label: "Verify No Mode", URL: "/c/ig/receive", Status: 400, Response: "unknown request"},
		{Label: "Verify No Secret", URL: "/c/ig/receive?hub.mode=subscribe", Status: 400, Response: "token does not match secret"},
		{Label: "Invalid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=blah", Status: 400, Response: "token does not match secret"},
		{Label: "Valid Secret", URL: "/c/ig/receive?hub.mode=subscribe&hub.verify_token=fb_webhook_secret&hub.challenge=yarchallenge", Status: 200, Response: "yarchallenge"},
	})

}

// setSendURL takes care of setting the send_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
	graphURL = s.URL
}

var SendTestCasesFBA = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Response",
		Text: "Simple Message", URN: "facebook:12345",
		Status: "W", ExternalID: "mid.133", ResponseToExternalID: "23526",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Send using ref URN",
		Text: "Simple Message", URN: "facebook:ref:67890",
		ContactURNs: map[string]bool{"facebook:12345": true, "ext:67890": true, "facebook:ref:67890": false},
		Status:      "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133", "recipient_id": "12345"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"user_ref":"67890"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "facebook:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "facebook:12345", QuickReplies: []string{"Yes", "No"}, Topic: "account",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"ACCOUNT_UPDATE","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Photo",
		URN: "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "facebook:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"}, Topic: "event",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"CONFIRMED_EVENT_UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Document",
		URN: "facebook:12345", Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "ID Error",
		Text: "ID Error", URN: "facebook:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error",
		Text: "Error", URN: "facebook:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

var SendTestCasesIG = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Plain Response",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133", ResponseToExternalID: "23526",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"RESPONSE","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Are you happy?", URN: "instagram:12345", QuickReplies: []string{"Yes", "No"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"Are you happy?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: "This is a long message which spans more than one part, what will actually be sent in the end if we exceed the max length?",
		URN:  "instagram:12345", QuickReplies: []string{"Yes", "No"}, Topic: "agent",
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"we exceed the max length?","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Send Photo",
		URN: "instagram:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"image","payload":{"url":"https://foo.bar/image.jpg","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "Send caption and photo with Quick Reply",
		Text: "This is some text.",
		URN:  "instagram:12345", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		QuickReplies: []string{"Yes", "No"},
		Status:       "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"text":"This is some text.","quick_replies":[{"title":"Yes","payload":"Yes","content_type":"text"},{"title":"No","payload":"No","content_type":"text"}]}}`,
		SendPrep:    setSendURL},
	{Label: "Tag Human Agent",
		Text: "Simple Message", URN: "instagram:12345",
		Status: "W", ExternalID: "mid.133", Topic: "agent",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"MESSAGE_TAG","tag":"HUMAN_AGENT","recipient":{"id":"12345"},"message":{"text":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Send Document",
		URN: "instagram:12345", Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Status: "W", ExternalID: "mid.133",
		ResponseBody: `{"message_id": "mid.133"}`, ResponseStatus: 200,
		RequestBody: `{"messaging_type":"UPDATE","recipient":{"id":"12345"},"message":{"attachment":{"type":"file","payload":{"url":"https://foo.bar/document.pdf","is_reusable":true}}}}`,
		SendPrep:    setSendURL},
	{Label: "ID Error",
		Text: "ID Error", URN: "instagram:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Error",
		Text: "Error", URN: "instagram:12345",
		Status:       "E",
		ResponseBody: `{ "is_error": true }`, ResponseStatus: 403,
		SendPrep: setSendURL},
}

var SendTestCasesWAC = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "whatsapp:250788123123", Path: "/12345_ID/messages",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"Simple Message"}}`,
		SendPrep:    setSendURL},
	{Label: "Unicode Send",
		Text: "☺", URN: "whatsapp:250788123123", Path: "/12345_ID/messages",
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"☺"}}`,
		SendPrep:    setSendURL},
	{Label: "Audio Send",
		Text:   "audio caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"audio/mpeg:https://foo.bar/audio.mp3"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"audio","audio":{"link":"https://foo.bar/audio.mp3"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"audio caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},
	{Label: "Document Send",
		Text:   "document caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"application/pdf:https://foo.bar/document.pdf"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"document","document":{"link":"https://foo.bar/document.pdf"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"document caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},

	{Label: "Image Send",
		Text:   "document caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"document caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},
	{Label: "Video Send",
		Text:   "video caption",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"video/mp4:https://foo.bar/video.mp4"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"video","video":{"link":"https://foo.bar/video.mp4"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"text","text":{"body":"video caption"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},

	{Label: "Template Send",
		Text:   "templated message",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Metadata:     json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "variables": ["Chef", "tomorrow"]}}`),
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 200,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		SendPrep:    setSendURL,
	},

	{Label: "Template Country Language",
		Text:   "templated message",
		URN:    "whatsapp:250788123123",
		Status: "W", ExternalID: "157b5e14568e8",
		Metadata:     json.RawMessage(`{ "templating": { "template": { "name": "revive_issue", "uuid": "171f8a4d-f725-46d7-85a6-11aceff0bfe3" }, "language": "eng", "country": "US", "variables": ["Chef", "tomorrow"]}}`),
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 200,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"template","template":{"name":"revive_issue","language":{"policy":"deterministic","code":"en_US"},"components":[{"type":"body","sub_type":"","index":"","parameters":[{"type":"text","text":"Chef"},{"type":"text","text":"tomorrow"}]}]}}`,
		SendPrep:    setSendURL,
	},
	{Label: "Template Invalid Language",
		Text: "templated message", URN: "whatsapp:250788123123",
		Error:    `unable to decode template: {"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}} for channel: 8eb23e93-5ecb-45ba-b726-3b064e0c56ab: unable to find mapping for language: bnt`,
		Metadata: json.RawMessage(`{"templating": { "template": { "name": "revive_issue", "uuid": "8ca114b4-bee2-4d3b-aaf1-9aa6b48d41e8" }, "language": "bnt", "variables": ["Chef", "tomorrow"]}}`),
	},
	{Label: "Interactive Button Message Send",
		Text: "Interactive Button Msg", URN: "whatsapp:250788123123", QuickReplies: []string{"BUTTON1"},
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
		SendPrep:    setSendURL},
	{Label: "Interactive List Message Send",
		Text: "Interactive List Msg", URN: "whatsapp:250788123123", QuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		Status: "W", ExternalID: "157b5e14568e8",
		ResponseBody: `{ "messages": [{"id": "157b5e14568e8"}] }`, ResponseStatus: 201,
		RequestBody: `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
		SendPrep:    setSendURL},
	{Label: "Interactive Button Message Send with attachment",
		Text: "Interactive Button Msg", URN: "whatsapp:250788123123", QuickReplies: []string{"BUTTON1"},
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"button","body":{"text":"Interactive Button Msg"},"action":{"buttons":[{"type":"reply","reply":{"id":"0","title":"BUTTON1"}}]}}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},
	{Label: "Interactive List Message Send with attachment",
		Text: "Interactive List Msg", URN: "whatsapp:250788123123", QuickReplies: []string{"ROW1", "ROW2", "ROW3", "ROW4"},
		Status: "W", ExternalID: "157b5e14568e8",
		Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Responses: map[MockedRequest]MockedResponse{
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"image","image":{"link":"https://foo.bar/image.jpg"}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
			MockedRequest{
				Method: "POST",
				Path:   "/12345_ID/messages",
				Body:   `{"messaging_product":"whatsapp","preview_url":false,"recipient_type":"individual","to":"250788123123","type":"interactive","interactive":{"type":"list","body":{"text":"Interactive List Msg"},"action":{"button":"Menu","sections":[{"rows":[{"id":"0","title":"ROW1"},{"id":"1","title":"ROW2"},{"id":"2","title":"ROW3"},{"id":"3","title":"ROW4"}]}]}}}`,
			}: MockedResponse{
				Status: 201,
				Body:   `{ "messages": [{"id": "157b5e14568e8"}] }`,
			},
		},
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100
	var ChannelFBA = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FBA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	var ChannelIG = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	var ChannelWAC = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "WAC", "12345_ID", "", map[string]interface{}{courier.ConfigAuthToken: "a123"})
	RunChannelSendTestCases(t, ChannelFBA, newHandler("FBA", "Facebook", false), SendTestCasesFBA, nil)
	RunChannelSendTestCases(t, ChannelIG, newHandler("IG", "Instagram", false), SendTestCasesIG, nil)
	RunChannelSendTestCases(t, ChannelWAC, newHandler("WAC", "Cloud API WhatsApp", false), SendTestCasesWAC, nil)
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"308de7627fe19e92294c4572a7f831bc1002809d",
		},
		{
			"hello world2",
			"ab6f902b58b9944032d4a960f470d7a8ebfd12b7",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}
