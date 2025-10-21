package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequestPrepFunc is our type for a hook for tests to use before a request is fired in a test
type RequestPrepFunc func(*http.Request)

// ExpectedStatus is an expected status update
type ExpectedStatus struct {
	MsgUUID    models.MsgUUID
	ExternalID string
	Status     models.MsgStatus

	MsgID models.MsgID // Deprecated: should be using MsgUUID
}

// ExpectedEvent is an expected channel event
type ExpectedEvent struct {
	Type  models.ChannelEventType
	URN   urns.URN
	Time  time.Time
	Extra map[string]string
}

// IncomingTestCase defines the test values for a particular test case
type IncomingTestCase struct {
	Label                 string
	NoQueueErrorCheck     bool
	NoInvalidChannelCheck bool
	PrepRequest           RequestPrepFunc

	URL           string
	Data          string
	Headers       map[string]string
	MultipartForm map[string]string

	ExpectedRespStatus    int
	ExpectedBodyContains  string
	ExpectedContactName   *string
	ExpectedMsgText       *string
	ExpectedURN           urns.URN
	ExpectedURNAuthTokens map[urns.URN]map[string]string
	ExpectedAttachments   []string
	ExpectedDate          time.Time
	ExpectedExternalID    string
	ExpectedMsgID         int64
	ExpectedStatuses      []ExpectedStatus
	ExpectedEvents        []ExpectedEvent
	ExpectedErrors        []*clogs.Error
	NoLogsExpected        bool
}

// utility method to make a request to a handler URL
func testHandlerRequest(tb testing.TB, s courier.Server, path string, headers map[string]string, data string, multipartFormFields map[string]string, expectedStatus int, expectedBodyContains string, requestPrepFunc RequestPrepFunc) string {
	var req *http.Request
	var err error
	url := fmt.Sprintf("https://%s%s", s.Config().Domain, path)

	if data != "" {
		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader(data))
		require.Nil(tb, err)

		// guess our content type
		contentType := "application/x-www-form-urlencoded"
		if strings.Contains(data, "{") && strings.Contains(data, "}") {
			contentType = "application/json"
		} else if strings.Contains(data, "<") && strings.Contains(data, ">") {
			contentType = "application/xml"
		}
		req.Header.Set("Content-Type", contentType)
	} else if multipartFormFields != nil {
		var body bytes.Buffer
		bodyMultipartWriter := multipart.NewWriter(&body)
		for k, v := range multipartFormFields {
			fieldWriter, err := bodyMultipartWriter.CreateFormField(k)
			require.Nil(tb, err)
			_, err = fieldWriter.Write([]byte(v))
			require.Nil(tb, err)
		}
		contentType := fmt.Sprintf("multipart/form-data;boundary=%v", bodyMultipartWriter.Boundary())
		bodyMultipartWriter.Close()

		req, err = http.NewRequest(http.MethodPost, url, bytes.NewReader(body.Bytes()))
		require.Nil(tb, err)
		req.Header.Set("Content-Type", contentType)
	} else {
		req, err = http.NewRequest(http.MethodGet, url, nil)
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}

	require.Nil(tb, err)

	if requestPrepFunc != nil {
		requestPrepFunc(req)
	}

	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	body := rr.Body.String()

	assert.Equal(tb, expectedStatus, rr.Code, "status code mismatch")

	if expectedBodyContains != "" {
		assert.Contains(tb, body, expectedBodyContains)
	}

	return body
}

func newServer(backend courier.Backend) courier.Server {
	// for benchmarks, log to null
	logger := slog.Default()
	log.SetOutput(io.Discard)

	cfg := runtime.NewDefaultConfig()
	cfg.FacebookWebhookSecret = "fb_webhook_secret"
	cfg.FacebookApplicationSecret = "fb_app_secret"
	cfg.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"

	return courier.NewServerWithLogger(cfg, backend, logger)

}

// RunIncomingTestCases runs all the passed in tests cases for the passed in channel configurations
func RunIncomingTestCases(t *testing.T, channels []courier.Channel, handler courier.ChannelHandler, testCases []IncomingTestCase) {
	mb := test.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	mockNow := dates.NewSequentialNow(time.Date(2025, 10, 13, 11, 20, 0, 0, time.UTC), time.Second)

	uuids.SetGenerator(uuids.NewSeededGenerator(1234, mockNow))
	defer uuids.SetGenerator(uuids.DefaultGenerator)

	dates.SetNowFunc(mockNow)
	defer dates.SetNowFunc(time.Now)

	for _, tc := range testCases {
		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			mb.Reset()

			testHandlerRequest(t, s, tc.URL, tc.Headers, tc.Data, tc.MultipartForm, tc.ExpectedRespStatus, tc.ExpectedBodyContains, tc.PrepRequest)

			if tc.ExpectedMsgText != nil || tc.ExpectedAttachments != nil {
				require.Len(mb.WrittenMsgs(), 1, "expected a msg to be written")
				msg := mb.WrittenMsgs()[0].(*test.MockMsg)

				if tc.ExpectedMsgText != nil {
					assert.Equal(t, *tc.ExpectedMsgText, msg.Text())
				}
				if len(tc.ExpectedAttachments) > 0 {
					assert.Equal(t, tc.ExpectedAttachments, msg.Attachments())
				}
				if !tc.ExpectedDate.IsZero() {
					assert.Equal(t, tc.ExpectedDate.Local(), msg.ReceivedOn().Local())
				}
				if tc.ExpectedExternalID != "" {
					assert.Equal(t, tc.ExpectedExternalID, msg.ExternalID())
				}
				assert.Equal(t, tc.ExpectedURN, msg.URN())
			} else {
				assert.Empty(t, mb.WrittenMsgs(), "unexpected msg written")
			}

			actualStatuses := mb.WrittenMsgStatuses()
			assert.Len(t, actualStatuses, len(tc.ExpectedStatuses), "unexpected number of status updates written")
			for i, expectedStatus := range tc.ExpectedStatuses {
				if (len(actualStatuses) - 1) < i {
					break
				}
				actualStatus := actualStatuses[i]

				assert.Equal(t, expectedStatus.MsgID, actualStatus.MsgID(), "msg id mismatch for update %d", i)
				assert.Equal(t, expectedStatus.MsgUUID, actualStatus.MsgUUID(), "msg uuid mismatch for update %d", i)
				assert.Equal(t, expectedStatus.ExternalID, actualStatus.ExternalID(), "external id mismatch for update %d", i)
				assert.Equal(t, expectedStatus.Status, actualStatus.Status(), "status value mismatch for update %d", i)
			}

			actualEvents := mb.WrittenChannelEvents()
			assert.Len(t, actualEvents, len(tc.ExpectedEvents), "unexpected number of events written")
			for i, expectedEvent := range tc.ExpectedEvents {
				if (len(actualEvents) - 1) < i {
					break
				}
				actualEvent := actualEvents[i]

				assert.Equal(t, expectedEvent.Type, actualEvent.EventType(), "event type mismatch for event %d", i)
				assert.Equal(t, expectedEvent.URN, actualEvent.URN(), "URN mismatch for event %d", i)
				assert.Equal(t, expectedEvent.Extra, actualEvent.Extra(), "extra mismatch for event %d", i)

				if !expectedEvent.Time.IsZero() {
					assert.Equal(t, expectedEvent.Time, actualEvent.OccurredOn())
				}
			}

			if tc.ExpectedContactName != nil {
				require.Equal(*tc.ExpectedContactName, mb.LastContactName())
			}

			assert.Equal(t, tc.ExpectedURNAuthTokens, mb.URNAuthTokens())

			// unless we know there won't be a log, check one was written
			if !tc.NoLogsExpected {
				if assert.Equal(t, 1, len(mb.WrittenChannelLogs()), "expected a channel log") {

					clog := mb.WrittenChannelLogs()[0]
					assert.Equal(t, append([]*clogs.Error{}, tc.ExpectedErrors...), clog.Errors, "unexpected errors logged")
				}
			}
		})
	}

	// check non-channel specific error conditions against first test case
	validCase := testCases[0]

	if !validCase.NoQueueErrorCheck {
		t.Run("Queue Error", func(t *testing.T) {
			mb.SetErrorOnQueue(true)
			defer mb.SetErrorOnQueue(false)
			testHandlerRequest(t, s, validCase.URL, validCase.Headers, validCase.Data, validCase.MultipartForm, 400, "unable to queue message", validCase.PrepRequest)
		})
	}

	if !validCase.NoInvalidChannelCheck {
		t.Run("Receive With Invalid Channel", func(t *testing.T) {
			mb.ClearChannels()
			testHandlerRequest(t, s, validCase.URL, validCase.Headers, validCase.Data, validCase.MultipartForm, 400, "channel not found", validCase.PrepRequest)
		})
	}
}

// SendPrepFunc allows test cases to modify the channel, msg or server before a message is sent
type SendPrepFunc func(*httptest.Server, courier.ChannelHandler, courier.Channel, courier.MsgOut)

type ExpectedRequest struct {
	Headers      map[string]string
	Path         string
	Params       url.Values
	Form         url.Values
	Body         string
	BodyContains string
}

func (e *ExpectedRequest) AssertMatches(t *testing.T, actual *http.Request, requestNum int) {
	if e.Headers != nil {
		for k, v := range e.Headers {
			assert.Equal(t, v, actual.Header.Get(k), "header %s mismatch for request %d", k, requestNum)
		}
	}
	if e.Path != "" {
		assert.Equal(t, e.Path, actual.URL.Path, "patch mismatch for request %d", requestNum)
	}
	if e.Params != nil {
		assert.Equal(t, e.Params, actual.URL.Query(), "URL params mismatch for request %d", requestNum)
	}
	if e.Form != nil {
		actual.ParseMultipartForm(32 << 20)
		assert.Equal(t, e.Form, actual.PostForm, "form mismatch for request %d", requestNum)
	}
	if e.Body != "" {
		value, _ := io.ReadAll(actual.Body)
		assert.Equal(t, e.Body, strings.Trim(string(value), "\n"), "body mismatch for request %d", requestNum)
	}
	if e.BodyContains != "" {
		value, _ := io.ReadAll(actual.Body)
		assert.Contains(t, string(value), e.BodyContains, "body contains fail for request %d", requestNum)
	}
}

// OutgoingTestCase defines the test values for a particular test case
type OutgoingTestCase struct {
	Label string

	MsgText                 string
	MsgURN                  string
	MsgURNAuth              string
	MsgAttachments          []string
	MsgQuickReplies         []models.QuickReply
	MsgLocale               i18n.Locale
	MsgTemplating           string
	MsgHighPriority         bool
	MsgResponseToExternalID string
	MsgFlow                 *models.FlowReference
	MsgOptIn                *models.OptInReference
	MsgUserID               models.UserID
	MsgOrigin               models.MsgOrigin
	MsgContactLastSeenOn    *time.Time

	MockResponses map[string][]*httpx.MockResponse

	ExpectedRequests    []ExpectedRequest
	ExpectedExtIDs      []string
	ExpectedError       error
	ExpectedLogErrors   []*clogs.Error
	ExpectedContactURNs map[string]bool
	ExpectedNewURN      string
}

// Msg creates the test message for this test case
func (tc *OutgoingTestCase) Msg(mb *test.MockBackend, ch courier.Channel) courier.MsgOut {
	msgOrigin := models.MsgOriginFlow
	if tc.MsgOrigin != "" {
		msgOrigin = tc.MsgOrigin
	}

	c := &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc", LastSeenOn: tc.MsgContactLastSeenOn}
	m := mb.NewOutgoingMsg(ch, "0191e180-7d60-7000-aded-7d8b151cbd5b", 10, c, urns.URN(tc.MsgURN), tc.MsgText, tc.MsgHighPriority, tc.MsgQuickReplies, tc.MsgResponseToExternalID, msgOrigin).(*test.MockMsg)
	m.WithLocale(tc.MsgLocale)
	m.WithUserID(tc.MsgUserID)

	for _, a := range tc.MsgAttachments {
		m.WithAttachment(a)
	}
	if tc.MsgURNAuth != "" {
		m.WithURNAuth(tc.MsgURNAuth)
	}
	if tc.MsgTemplating != "" {
		templating := &models.Templating{}
		jsonx.MustUnmarshal([]byte(tc.MsgTemplating), templating)
		m.WithTemplating(templating)
	}
	if tc.MsgFlow != nil {
		m.WithFlow(tc.MsgFlow)
	}
	if tc.MsgOptIn != nil {
		m.WithOptIn(tc.MsgOptIn)
	}
	return m
}

// RunOutgoingTestCases runs all the passed in test cases against the channel
func RunOutgoingTestCases(t *testing.T, channel courier.Channel, handler courier.ChannelHandler, testCases []OutgoingTestCase, checkRedacted []string, setupBackend func(*test.MockBackend)) {
	mb := test.NewMockBackend()
	if setupBackend != nil {
		setupBackend(mb)
	}
	s := newServer(mb)
	mb.AddChannel(channel)
	handler.Initialize(s)

	for _, tc := range testCases {
		mb.Reset()

		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			msg := tc.Msg(mb, channel)

			var mockHTTP *httpx.MockRequestor
			actualRequests := make([]*http.Request, 0, 1)

			if len(tc.MockResponses) > 0 {
				mockHTTP = httpx.NewMockRequestor(tc.MockResponses).Clone()
				httpx.SetRequestor(mockHTTP)
			}

			clog := courier.NewChannelLogForSend(msg, handler.RedactValues(channel))
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)

			res := &courier.SendResult{}
			serr := handler.Send(ctx, msg, res, clog)
			externalIDs := res.ExternalIDs()
			resNewURN := res.GetNewURN()

			if mockHTTP != nil {
				httpx.SetRequestor(httpx.DefaultRequestor)

				actualRequests = mockHTTP.Requests()

				assert.False(t, mockHTTP.HasUnused(), "unused HTTP mocks")
			}

			cancel()

			if len(tc.ExpectedRequests) > 0 {
				assert.Len(t, actualRequests, len(tc.ExpectedRequests), "unexpected number of requests made")

				for i, expectedRequest := range tc.ExpectedRequests {
					if (len(actualRequests) - 1) < i {
						break
					}
					expectedRequest.AssertMatches(t, actualRequests[i], i)
				}
			}

			assert.Equal(t, tc.ExpectedExtIDs, externalIDs, "external IDs mismatch")
			assert.Equal(t, tc.ExpectedError, serr, "send method error mismatch")
			assert.Equal(t, append([]*clogs.Error{}, tc.ExpectedLogErrors...), clog.Errors, "channel log errors mismatch")

			if tc.ExpectedContactURNs != nil {
				var contactUUID models.ContactUUID
				for urn, shouldBePresent := range tc.ExpectedContactURNs {
					contact, _ := mb.GetContact(ctx, channel, urns.URN(urn), nil, "", true, clog)
					if contactUUID == models.NilContactUUID && shouldBePresent {
						contactUUID = contact.UUID()
					}
					if shouldBePresent {
						require.Equal(contactUUID, contact.UUID())
					} else {
						require.NotEqual(contactUUID, contact.UUID())
					}

				}
			}

			if tc.ExpectedNewURN != "" {
				require.Equal(urns.URN(tc.ExpectedNewURN), resNewURN)
			}

			AssertChannelLogRedaction(t, clog, checkRedacted)
		})
	}
}

// asserts that the given channel log doesn't contain any of the given values
func AssertChannelLogRedaction(t *testing.T, clog *courier.ChannelLog, vals []string) {
	assertRedacted := func(s string) {
		for _, v := range vals {
			assert.NotContains(t, s, v, "expected '%s' to not contain redacted value '%s'", s, v)
		}
	}

	for _, h := range clog.HttpLogs {
		assertRedacted(h.URL)
		assertRedacted(h.Request)
		assertRedacted(h.Response)
	}
	for _, e := range clog.Errors {
		assertRedacted(e.Message)
	}
}

// Sp is a utility method to get the pointer to the passed in string
func Sp(s string) *string { return &s }
