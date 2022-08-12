package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fmt"

	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequestPrepFunc is our type for a hook for tests to use before a request is fired in a test
type RequestPrepFunc func(*http.Request)

// ChannelHandleTestCase defines the test values for a particular test case
type ChannelHandleTestCase struct {
	Label string

	URL                 string
	Data                string
	Headers             map[string]string
	MultipartFormFields map[string]string

	ExpectedStatus   int
	ExpectedResponse string

	ExpectedContactName *string
	ExpectedMsgText     *string
	ExpectedURN         *string
	ExpectedURNAuth     *string
	ExpectedAttachments []string
	ExpectedDate        *time.Time
	ExpectedMsgStatus   *string
	ExpectedExternalID  *string
	ExpectedMsgID       int64

	ChannelEvent      *string
	ChannelEventExtra map[string]interface{}

	NoQueueErrorCheck     bool
	NoInvalidChannelCheck bool

	PrepRequest RequestPrepFunc
}

// SendPrepFunc allows test cases to modify the channel, msg or server before a message is sent
type SendPrepFunc func(*httptest.Server, courier.ChannelHandler, courier.Channel, courier.Msg)

// MockedRequest is a fake HTTP request
type MockedRequest struct {
	Method       string
	Path         string
	RawQuery     string
	Body         string
	BodyContains string
}

// ChannelSendTestCase defines the test values for a particular test case
type ChannelSendTestCase struct {
	Label    string
	SendPrep SendPrepFunc

	MsgText                 string
	MsgURN                  string
	MsgURNAuth              string
	MsgAttachments          []string
	MsgQuickReplies         []string
	MsgTopic                string
	MsgHighPriority         bool
	MsgResponseToExternalID string
	MsgMetadata             json.RawMessage
	MsgFlow                 *courier.FlowReference

	MockResponseStatus int
	MockResponseBody   string
	MockResponses      map[MockedRequest]*httpx.MockResponse

	ExpectedRequestPath string
	ExpectedURLParams   map[string]string
	ExpectedPostParams  map[string]string
	ExpectedRequestBody string
	ExpectedHeaders     map[string]string

	ExpectedStatus     string
	ExpectedExternalID string
	ExpectedErrors     []string

	ExpectedStopEvent   bool
	ExpectedContactURNs map[string]bool
	ExpectedNewURN      string
}

// Sp is a utility method to get the pointer to the passed in string
func Sp(str interface{}) *string { asStr := fmt.Sprintf("%s", str); return &asStr }

// Tp is utility method to get the pointer to the passed in time
func Tp(tm time.Time) *time.Time { return &tm }

// utility method to make sure the passed in host is up, prevents races with our test server
func ensureTestServerUp(host string) {
	for i := 0; i < 20; i++ {
		_, err := http.Get(host)
		if err == nil {
			break
		}
		time.Sleep(time.Microsecond * 100)
	}
}

// utility method to make a request to a handler URL
func testHandlerRequest(tb testing.TB, s courier.Server, path string, headers map[string]string, data string, multipartFormFields map[string]string, expectedStatus int, expectedBody *string, requestPrepFunc RequestPrepFunc) string {
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

	if headers != nil {
		for key, val := range headers {
			req.Header.Set(key, val)
		}
	}

	require.Nil(tb, err)

	if requestPrepFunc != nil {
		requestPrepFunc(req)
	}

	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	body := rr.Body.String()

	require.Equal(tb, expectedStatus, rr.Code, fmt.Sprintf("incorrect status code with response: %s", body))

	if expectedBody != nil {
		require.Contains(tb, body, *expectedBody)
	}

	return body
}

func newServer(backend courier.Backend) courier.Server {
	// for benchmarks, log to null
	logger := logrus.New()
	logger.Out = ioutil.Discard
	logrus.SetOutput(ioutil.Discard)

	config := courier.NewConfig()
	config.FacebookWebhookSecret = "fb_webhook_secret"
	config.FacebookApplicationSecret = "fb_app_secret"
	config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"

	return courier.NewServerWithLogger(config, backend, logger)

}

// RunChannelSendTestCases runs all the passed in test cases against the channel
func RunChannelSendTestCases(t *testing.T, channel courier.Channel, handler courier.ChannelHandler, testCases []ChannelSendTestCase, setupBackend func(*test.MockBackend)) {
	mb := test.NewMockBackend()
	if setupBackend != nil {
		setupBackend(mb)
	}
	s := newServer(mb)
	mb.AddChannel(channel)
	handler.Initialize(s)

	for _, tc := range testCases {
		mockRRCount := 0
		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			msg := mb.NewOutgoingMsg(channel, courier.NewMsgID(10), urns.URN(tc.MsgURN), tc.MsgText, tc.MsgHighPriority, tc.MsgQuickReplies, tc.MsgTopic, tc.MsgResponseToExternalID)

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
				body, _ := ioutil.ReadAll(r.Body)
				testRequest = httptest.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(body))
				testRequest.Header = r.Header
				if (len(tc.MockResponses)) == 0 {
					w.WriteHeader(tc.MockResponseStatus)
					w.Write([]byte(tc.MockResponseBody))
				} else {
					require.Zero(tc.MockResponseStatus, "ResponseStatus should not be used when using testcase.Responses")
					require.Zero(tc.MockResponseBody, "ResponseBody should not be used when using testcase.Responses")
					for mockRequest, mockResponse := range tc.MockResponses {
						bodyStr := string(body)[:]
						if mockRequest.Method == r.Method && mockRequest.Path == r.URL.Path && mockRequest.RawQuery == r.URL.RawQuery && (mockRequest.Body == bodyStr || (mockRequest.BodyContains != "" && strings.Contains(bodyStr, mockRequest.BodyContains))) {
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

			logger := courier.NewChannelLoggerForSend(msg)

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
			status, err := handler.Send(ctx, msg, logger)
			cancel()

			// we don't currently distinguish between a returned error and logged errors
			if err != nil {
				logger.Error(err)
			}

			assert.Equal(t, tc.ExpectedErrors, logger.Errors(), "unexpected errors logged")

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
			}

			if tc.ExpectedRequestBody != "" {
				require.NotNil(testRequest, "request body should not be nil")
				value, _ := ioutil.ReadAll(testRequest.Body)
				require.Equal(tc.ExpectedRequestBody, strings.Trim(string(value), "\n"))
			}

			if (len(tc.MockResponses)) != 0 {
				require.Equal(mockRRCount, len(tc.MockResponses))
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

			if tc.ExpectedStatus != "" {
				require.NotNil(status, "status should not be nil")
				require.Equal(tc.ExpectedStatus, string(status.Status()))
			}

			if tc.ExpectedStopEvent {
				evt, err := mb.GetLastChannelEvent()
				require.NoError(err)
				require.Equal(courier.StopContact, evt.EventType())
			}

			if tc.ExpectedContactURNs != nil {
				var contactUUID courier.ContactUUID
				for urn, shouldBePresent := range tc.ExpectedContactURNs {
					contact, _ := mb.GetContact(ctx, channel, urns.URN(urn), "", "")
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
				old, new := status.UpdatedURN()
				require.Equal(urns.URN(tc.MsgURN), old)
				require.Equal(urns.URN(tc.ExpectedNewURN), new)
			}
		})
	}

}

// RunChannelTestCases runs all the passed in tests cases for the passed in channel configurations
func RunChannelTestCases(t *testing.T, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelHandleTestCase) {
	mb := test.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, tc := range testCases {
		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			mb.ClearQueueMsgs()
			mb.ClearSeenExternalIDs()

			testHandlerRequest(t, s, tc.URL, tc.Headers, tc.Data, tc.MultipartFormFields, tc.ExpectedStatus, &tc.ExpectedResponse, tc.PrepRequest)

			// pop our message off and test against it
			contactName := mb.GetLastContactName()
			msg, _ := mb.GetLastQueueMsg()
			event, _ := mb.GetLastChannelEvent()
			status, _ := mb.GetLastMsgStatus()

			if tc.ExpectedStatus == 200 {
				if tc.ExpectedContactName != nil {
					require.Equal(*tc.ExpectedContactName, contactName)
				}
				if tc.ExpectedMsgText != nil {
					require.NotNil(msg)
					require.Equal(mb.LenQueuedMsgs(), 1)
					require.Equal(*tc.ExpectedMsgText, msg.Text())
				}
				if tc.ChannelEvent != nil {
					require.Equal(*tc.ChannelEvent, string(event.EventType()))
				}
				if tc.ChannelEventExtra != nil {
					require.Equal(tc.ChannelEventExtra, event.Extra())
				}
				if tc.ExpectedURN != nil {
					if msg != nil {
						require.Equal(*tc.ExpectedURN, string(msg.URN()))
					} else if event != nil {
						require.Equal(*tc.ExpectedURN, string(event.URN()))
					} else {
						require.Equal(*tc.ExpectedURN, "")
					}
				}
				if tc.ExpectedURNAuth != nil {
					if msg != nil {
						require.Equal(*tc.ExpectedURNAuth, msg.URNAuth())
					}
				}
				if tc.ExpectedExternalID != nil {
					if msg != nil {
						require.Equal(*tc.ExpectedExternalID, msg.ExternalID())
					} else if status != nil {
						require.Equal(*tc.ExpectedExternalID, status.ExternalID())
					} else {
						require.Equal(*tc.ExpectedExternalID, "")
					}
				}
				if tc.ExpectedMsgStatus != nil {
					require.NotNil(status)
					require.Equal(*tc.ExpectedMsgStatus, string(status.Status()))
				}
				if tc.ExpectedMsgID != 0 {
					if status != nil {
						require.Equal(tc.ExpectedMsgID, int64(status.ID()))
					} else {
						require.Equal(tc.ExpectedMsgID, -1)
					}
				}
				if len(tc.ExpectedAttachments) > 0 {
					require.Equal(tc.ExpectedAttachments, msg.Attachments())
				}
				if tc.ExpectedDate != nil {
					if msg != nil {
						require.Equal((*tc.ExpectedDate).Local(), (*msg.ReceivedOn()).Local())
					} else if event != nil {
						require.Equal(*tc.ExpectedDate, event.OccurredOn())
					} else {
						require.Equal(*tc.ExpectedDate, nil)
					}
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
			testHandlerRequest(t, s, validCase.URL, validCase.Headers, validCase.Data, validCase.MultipartFormFields, 400, Sp("unable to queue message"), validCase.PrepRequest)
		})
	}

	if !validCase.NoInvalidChannelCheck {
		t.Run("Receive With Invalid Channel", func(t *testing.T) {
			mb.ClearChannels()
			testHandlerRequest(t, s, validCase.URL, validCase.Headers, validCase.Data, validCase.MultipartFormFields, 400, Sp("channel not found"), validCase.PrepRequest)
		})
	}
}

// RunChannelBenchmarks runs all the passed in test cases for the passed in channels
func RunChannelBenchmarks(b *testing.B, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelHandleTestCase) {
	mb := test.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		mb.ClearQueueMsgs()
		mb.ClearSeenExternalIDs()

		b.Run(testCase.Label, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				testHandlerRequest(b, s, testCase.URL, testCase.Headers, testCase.Data, testCase.MultipartFormFields, testCase.ExpectedStatus, nil, testCase.PrepRequest)
			}
		})
	}
}
