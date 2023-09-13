package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequestPrepFunc is our type for a hook for tests to use before a request is fired in a test
type RequestPrepFunc func(*http.Request)

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
	ExpectedURNAuthTokens map[string]string
	ExpectedAttachments   []string
	ExpectedDate          time.Time
	ExpectedMsgStatus     courier.MsgStatus
	ExpectedExternalID    string
	ExpectedMsgID         int64
	ExpectedEvent         courier.ChannelEventType
	ExpectedEventExtra    map[string]string
	ExpectedErrors        []*courier.ChannelError
	NoLogsExpected        bool
}

// MockedRequest is a fake HTTP request
type MockedRequest struct {
	Method       string
	Path         string
	RawQuery     string
	Body         string
	BodyContains string
}

func (m MockedRequest) Matches(r *http.Request, body []byte) bool {
	return m.Method == r.Method && m.Path == r.URL.Path && m.RawQuery == r.URL.RawQuery && (m.Body == string(body) || (m.BodyContains != "" && strings.Contains(string(body), m.BodyContains)))
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
	logger := logrus.New()
	logger.Out = io.Discard
	logrus.SetOutput(io.Discard)

	config := courier.NewConfig()
	config.FacebookWebhookSecret = "fb_webhook_secret"
	config.FacebookApplicationSecret = "fb_app_secret"
	config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"

	return courier.NewServerWithLogger(config, backend, logger)

}

// RunIncomingTestCases runs all the passed in tests cases for the passed in channel configurations
func RunIncomingTestCases(t *testing.T, channels []courier.Channel, handler courier.ChannelHandler, testCases []IncomingTestCase) {
	mb := test.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

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
				assert.Equal(t, tc.ExpectedURNAuthTokens, msg.URNAuthTokens())
			} else {
				assert.Empty(t, mb.WrittenMsgs(), "unexpected msg written")
			}

			if tc.ExpectedMsgStatus != "" {
				// TODO find better way to test statuses because some channels (e.g. infobip) can create multiple statuses in one call
				require.Greater(len(mb.WrittenMsgStatuses()), 0, "expected a msg status to be written")
				status := mb.WrittenMsgStatuses()[len(mb.WrittenMsgStatuses())-1]

				assert.Equal(t, tc.ExpectedMsgStatus, status.Status())

				if tc.ExpectedExternalID != "" {
					assert.Equal(t, tc.ExpectedExternalID, status.ExternalID())
				}
				if tc.ExpectedMsgID != 0 {
					assert.Equal(t, tc.ExpectedMsgID, int64(status.MsgID()))
				}
			} else {
				assert.Empty(t, mb.WrittenMsgStatuses(), "unexpected msg status written")
			}

			if tc.ExpectedEvent != "" {
				require.Len(mb.WrittenChannelEvents(), 1, "expected a channel event to be written")
				event := mb.WrittenChannelEvents()[0]

				assert.Equal(t, tc.ExpectedEvent, event.EventType())
				assert.Equal(t, tc.ExpectedEventExtra, event.Extra())
				assert.Equal(t, tc.ExpectedURN, event.URN())

				if !tc.ExpectedDate.IsZero() {
					assert.Equal(t, tc.ExpectedDate, event.OccurredOn())
				}
			} else {
				assert.Empty(t, mb.WrittenChannelEvents(), "unexpected channel event written")
			}

			if tc.ExpectedContactName != nil {
				require.Equal(*tc.ExpectedContactName, mb.LastContactName())
			}

			// unless we know there won't be a log, check one was written
			if !tc.NoLogsExpected {
				if assert.Equal(t, 1, len(mb.WrittenChannelLogs()), "expected a channel log") {

					clog := mb.WrittenChannelLogs()[0]
					assert.Equal(t, tc.ExpectedErrors, clog.Errors(), "unexpected errors logged")
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
type SendPrepFunc func(*httptest.Server, courier.ChannelHandler, courier.Channel, courier.Msg)

// OutgoingTestCase defines the test values for a particular test case
type OutgoingTestCase struct {
	Label    string
	SendPrep SendPrepFunc

	MsgText                 string
	MsgURN                  string
	MsgURNAuth              string
	MsgAttachments          []string
	MsgQuickReplies         []string
	MsgLocale               i18n.Locale
	MsgTopic                string
	MsgHighPriority         bool
	MsgResponseToExternalID string
	MsgMetadata             json.RawMessage
	MsgFlow                 *courier.FlowReference
	MsgOrigin               courier.MsgOrigin
	MsgContactLastSeenOn    *time.Time

	MockResponseStatus int
	MockResponseBody   string
	MockResponses      map[MockedRequest]*httpx.MockResponse

	ExpectedRequestPath string
	ExpectedURLParams   map[string]string
	ExpectedPostParams  map[string]string // deprecated, use ExpectedPostForm
	ExpectedPostForm    url.Values
	ExpectedRequestBody string
	ExpectedHeaders     map[string]string
	ExpectedMsgStatus   courier.MsgStatus
	ExpectedExternalID  string
	ExpectedErrors      []*courier.ChannelError
	ExpectedStopEvent   bool
	ExpectedContactURNs map[string]bool
	ExpectedNewURN      string
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
		mockRRCount := 0
		msgOrigin := courier.MsgOriginFlow
		if tc.MsgOrigin != "" {
			msgOrigin = tc.MsgOrigin
		}

		mb.Reset()

		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			msg := mb.NewOutgoingMsg(channel, 10, urns.URN(tc.MsgURN), tc.MsgText, tc.MsgHighPriority, tc.MsgQuickReplies, tc.MsgTopic, tc.MsgResponseToExternalID, msgOrigin, tc.MsgContactLastSeenOn).(*test.MockMsg)
			msg.WithLocale(tc.MsgLocale)

			for _, a := range tc.MsgAttachments {
				msg.WithAttachment(a)
			}
			if tc.MsgURNAuth != "" {
				msg.WithURNAuth(tc.MsgURNAuth)
			}
			if len(tc.MsgMetadata) > 0 {
				msg.WithMetadata(tc.MsgMetadata)
			}
			if tc.MsgFlow != nil {
				msg.WithFlow(tc.MsgFlow)
			}

			var testRequest *http.Request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				testRequest = httptest.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(body))
				testRequest.Header = r.Header

				if (len(tc.MockResponses)) == 0 {
					w.WriteHeader(tc.MockResponseStatus)
					w.Write([]byte(tc.MockResponseBody))
				} else {
					for mockRequest, mockResponse := range tc.MockResponses {
						if mockRequest == (MockedRequest{}) || mockRequest.Matches(r, body) {
							w.WriteHeader(mockResponse.Status)
							w.Write(mockResponse.Body)
							mockRRCount++
							break
						}
					}
				}
			}))
			defer server.Close()

			// call our prep function if we have one
			if tc.SendPrep != nil {
				tc.SendPrep(server, handler, channel, msg)
			}

			clog := courier.NewChannelLogForSend(msg, handler.RedactValues(channel))

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
			status, err := handler.Send(ctx, msg, clog)
			cancel()

			// sender adds returned error to channel log if there aren't other logged errors
			if err != nil && len(clog.Errors()) == 0 {
				clog.RawError(err)
			}

			assert.Equal(t, tc.ExpectedErrors, clog.Errors(), "unexpected errors logged")

			if tc.ExpectedRequestPath != "" {
				require.NotNil(testRequest, "path should not be nil")
				require.Equal(tc.ExpectedRequestPath, testRequest.URL.Path)
			}

			if tc.ExpectedURLParams != nil {
				require.NotNil(testRequest)
				for k, v := range tc.ExpectedURLParams {
					value := testRequest.URL.Query().Get(k)
					require.Equal(v, value, fmt.Sprintf("%s not equal", k))
				}
			}

			if tc.ExpectedPostParams != nil {
				require.NotNil(testRequest, "post body should not be nil")
				for k, v := range tc.ExpectedPostParams {
					value := testRequest.PostFormValue(k)
					require.Equal(v, value)
				}
			} else if tc.ExpectedPostForm != nil {
				require.NotNil(testRequest, "post body should not be nil")
				testRequest.ParseMultipartForm(32 << 20)
				assert.Equal(t, tc.ExpectedPostForm, testRequest.PostForm)
			}

			if tc.ExpectedRequestBody != "" {
				require.NotNil(testRequest, "request body should not be nil")
				value, _ := io.ReadAll(testRequest.Body)
				require.Equal(tc.ExpectedRequestBody, strings.Trim(string(value), "\n"))
			}

			if (len(tc.MockResponses)) != 0 {
				assert.Equal(t, len(tc.MockResponses), mockRRCount, "mocked request count mismatch")
			}

			if tc.ExpectedHeaders != nil {
				require.NotNil(testRequest, "headers should not be nil")
				for k, v := range tc.ExpectedHeaders {
					value := testRequest.Header.Get(k)
					require.Equal(v, value)
				}
			}

			if tc.ExpectedExternalID != "" {
				require.Equal(tc.ExpectedExternalID, status.ExternalID())
			}

			if tc.ExpectedMsgStatus != "" {
				require.NotNil(status, "status should not be nil")
				require.Equal(tc.ExpectedMsgStatus, status.Status())
			}

			if tc.ExpectedStopEvent {
				require.Len(mb.WrittenChannelEvents(), 1)
				event := mb.WrittenChannelEvents()[0]
				require.Equal(courier.StopContact, event.EventType())
			}

			if tc.ExpectedContactURNs != nil {
				var contactUUID courier.ContactUUID
				for urn, shouldBePresent := range tc.ExpectedContactURNs {
					contact, _ := mb.GetContact(ctx, channel, urns.URN(urn), nil, "", clog)
					if contactUUID == courier.NilContactUUID && shouldBePresent {
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
				old, new := status.URNUpdate()
				require.Equal(urns.URN(tc.MsgURN), old)
				require.Equal(urns.URN(tc.ExpectedNewURN), new)
			}

			AssertChannelLogRedaction(t, clog, checkRedacted)
		})
	}
}

// RunChannelBenchmarks runs all the passed in test cases for the passed in channels
func RunChannelBenchmarks(b *testing.B, channels []courier.Channel, handler courier.ChannelHandler, testCases []IncomingTestCase) {
	mb := test.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		mb.Reset()

		b.Run(testCase.Label, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				testHandlerRequest(b, s, testCase.URL, testCase.Headers, testCase.Data, testCase.MultipartForm, testCase.ExpectedRespStatus, "", testCase.PrepRequest)
			}
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

	for _, h := range clog.HTTPLogs() {
		assertRedacted(h.URL)
		assertRedacted(h.Request)
		assertRedacted(h.Response)
	}
	for _, e := range clog.Errors() {
		assertRedacted(e.Message())
	}
}

// Sp is a utility method to get the pointer to the passed in string
func Sp(s string) *string { return &s }
