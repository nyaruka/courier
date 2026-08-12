package handlertest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/svclogs"
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
	ExpectedPayload       string
	ExpectedDate          time.Time
	ExpectedExternalID    string
	ExpectedStatuses      []ExpectedStatus
	ExpectedEvents        []ExpectedEvent
	ExpectedErrors        []*svclogs.Error
	ExpectedNewURN        *models.NewURNSpec
	NoLogsExpected        bool
}

// utility method to make a request to a handler URL
func testHandlerRequest(tb testing.TB, s *web.Server, path string, headers map[string]string, data string, multipartFormFields map[string]string, expectedStatus int, expectedBodyContains string, requestPrepFunc RequestPrepFunc) string {
	var req *http.Request
	var err error
	url := fmt.Sprintf("https://%s%s", s.Runtime().Config.Domain, path)

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

// localOnlyTransport passes through requests to test servers on this host and rejects everything else
type localOnlyTransport struct {
	http.RoundTripper
}

func (t localOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if host := r.URL.Hostname(); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("handler tests can't make requests outside this host: %s", r.URL)
	}
	return t.RoundTripper.RoundTrip(r)
}

func newServer(rt *runtime.Runtime) *web.Server {
	// for benchmarks, log to null
	log.SetOutput(io.Discard)

	rt.Config.FacebookWebhookSecret = "fb_webhook_secret"
	rt.Config.FacebookApplicationSecret = "fb_app_secret"
	rt.Config.WhatsappAdminSystemUserToken = "wac_admin_system_user_token"

	return web.NewServer(rt)
}

// RunIncomingTestCases runs all the passed in tests cases for the passed in channel configurations
func RunIncomingTestCases(t *testing.T, chs []*models.Channel, handler channels.Handler, testCases []IncomingTestCase) {
	_, rt := testsuite.Runtime(t)

	// state is reset once for the whole run rather than per case because handlers can carry state between
	// requests, e.g. those which join the parts of a multi-part message via valkey
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	// handlers reach test servers on localhost so use a plain client rather than the runtime's, whose
	// transport enforces the SSRF blocklist - but fail anything leaving the host so that a handler which
	// isn't pointed at a test server (e.g. one describing a URN whilst a contact is created) is caught
	// here rather than calling a real channel API
	client := &http.Client{Transport: httpx.WithTraces(localOnlyTransport{http.DefaultTransport}), Timeout: 30 * time.Second}
	rt.HTTP.Default = client
	rt.HTTP.Proxied = client
	rt.HTTP.Attachments = client

	// data: attachments are saved to storage as they're received so ensure the bucket exists
	rt.S3.Client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String(rt.Config.S3AttachmentsBucket)})

	s := newServer(rt)

	for _, ch := range chs {
		testsuite.InsertChannel(t, rt, ch)
	}

	// re-register the handler under test so that lookups by channel type - e.g. the URN describer used when
	// creating a contact - resolve to this instance rather than the uninitialized one from its init()
	channels.RegisterHandler(handler)
	require.NoError(t, s.MountHandler(handler))

	// capture the events and channel logs of each handled request
	var handledEvents []channels.Event
	var handledLogs []*models.ChannelLog
	s.OnRequestHandled(func(ch *models.Channel, evts []channels.Event, clog *models.ChannelLog) {
		handledEvents = append(handledEvents, evts...)
		handledLogs = append(handledLogs, clog)
	})

	mockNow := dates.NewSequentialNow(time.Date(2025, 10, 13, 11, 20, 0, 0, time.UTC), time.Second)

	uuids.SetGenerator(uuids.NewSeededGenerator(1234, mockNow))
	defer uuids.SetGenerator(uuids.DefaultGenerator)

	dates.SetNowFunc(mockNow)
	defer dates.SetNowFunc(time.Now)

	for _, tc := range testCases {
		t.Run(tc.Label, func(t *testing.T) {
			require := require.New(t)

			handledEvents = nil
			handledLogs = nil

			testHandlerRequest(t, s, tc.URL, tc.Headers, tc.Data, tc.MultipartForm, tc.ExpectedRespStatus, tc.ExpectedBodyContains, tc.PrepRequest)

			// organize the events the handler returned by type
			var msgs []*models.MsgIn
			var statuses []*models.StatusUpdate
			var events []*models.ChannelEvent
			lastContactName := ""
			var urnAuthTokens map[urns.URN]map[string]string

			for _, event := range handledEvents {
				switch e := event.(type) {
				case *models.MsgIn:
					msgs = append(msgs, e)
					lastContactName = e.ContactName_
					if e.URNAuthTokens_ != nil {
						if urnAuthTokens == nil {
							urnAuthTokens = make(map[urns.URN]map[string]string)
						}
						if urnAuthTokens[e.URN_] == nil {
							urnAuthTokens[e.URN_] = make(map[string]string)
						}
						for k, v := range e.URNAuthTokens_ {
							urnAuthTokens[e.URN_][k] = v
						}
					}
				case *models.StatusUpdate:
					statuses = append(statuses, e)
				case *models.ChannelEvent:
					events = append(events, e)
					lastContactName = e.ContactName_
				}
			}

			if tc.ExpectedMsgText != nil || tc.ExpectedAttachments != nil {
				require.Len(msgs, 1, "expected a msg to be written")
				msg := msgs[0]

				if tc.ExpectedMsgText != nil {
					assert.Equal(t, *tc.ExpectedMsgText, msg.Text())
				}
				if len(tc.ExpectedAttachments) > 0 {
					assert.Equal(t, tc.ExpectedAttachments, msg.Attachments())
				}
				if tc.ExpectedPayload != "" {
					assert.JSONEq(t, tc.ExpectedPayload, string(msg.Payload_))
				} else {
					assert.Nil(t, msg.Payload_)
				}
				if !tc.ExpectedDate.IsZero() {
					assert.Equal(t, tc.ExpectedDate.Local(), msg.ReceivedOn().Local())
				}
				if tc.ExpectedExternalID != "" {
					assert.Equal(t, tc.ExpectedExternalID, msg.ExternalID())
				}
				assert.Equal(t, tc.ExpectedURN, msg.URN())

				if tc.ExpectedNewURN != nil {
					assert.Equal(t, tc.ExpectedNewURN, msg.NewURN_, "new URN mismatch")
				} else {
					assert.Nil(t, msg.NewURN_, "unexpected new URN on message")
				}
			} else {
				assert.Empty(t, msgs, "unexpected msg written")
			}

			assert.Len(t, statuses, len(tc.ExpectedStatuses), "unexpected number of status updates written")
			for i, expectedStatus := range tc.ExpectedStatuses {
				if (len(statuses) - 1) < i {
					break
				}
				actualStatus := statuses[i]

				assert.Equal(t, expectedStatus.MsgUUID, actualStatus.MsgUUID(), "msg uuid mismatch for update %d", i)
				assert.Equal(t, expectedStatus.ExternalID, actualStatus.ExternalIdentifier(), "external identifier mismatch for update %d", i)
				assert.Equal(t, expectedStatus.Status, actualStatus.Status(), "status value mismatch for update %d", i)
			}

			assert.Len(t, events, len(tc.ExpectedEvents), "unexpected number of events written")
			for i, expectedEvent := range tc.ExpectedEvents {
				if (len(events) - 1) < i {
					break
				}
				actualEvent := events[i]

				assert.Equal(t, expectedEvent.Type, actualEvent.EventType(), "event type mismatch for event %d", i)
				assert.Equal(t, expectedEvent.URN, actualEvent.URN(), "URN mismatch for event %d", i)
				assert.Equal(t, expectedEvent.Extra, actualEvent.Extra(), "extra mismatch for event %d", i)

				if !expectedEvent.Time.IsZero() {
					assert.Equal(t, expectedEvent.Time, actualEvent.OccurredOn())
				}
			}

			if tc.ExpectedContactName != nil {
				require.Equal(*tc.ExpectedContactName, lastContactName)
			}

			assert.Equal(t, tc.ExpectedURNAuthTokens, urnAuthTokens)

			// unless we know there won't be a log, check one was written
			if !tc.NoLogsExpected {
				if assert.Equal(t, 1, len(handledLogs), "expected a channel log") {
					clog := handledLogs[0]
					assert.Equal(t, append([]*svclogs.Error{}, tc.ExpectedErrors...), clog.Errors, "unexpected errors logged")
				}
			}
		})
	}

	// check invalid channel condition against first test case
	validCase := testCases[0]

	if !validCase.NoInvalidChannelCheck {
		t.Run("Receive With Invalid Channel", func(t *testing.T) {
			for _, ch := range chs {
				rt.DB.MustExec(`DELETE FROM channels_channel WHERE uuid = $1`, ch.UUID())
			}
			models.FlushChannelCache()
			testHandlerRequest(t, s, validCase.URL, validCase.Headers, validCase.Data, validCase.MultipartForm, 400, "channel not found", validCase.PrepRequest)
		})
	}
}

// SendPrepFunc allows test cases to modify the channel, msg or server before a message is sent
type SendPrepFunc func(*httptest.Server, channels.Handler, *models.Channel, *models.MsgOut)

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
	MsgUserID               models.UserID
	MsgOrigin               models.MsgOrigin
	MsgContactLastSeenOn    *time.Time
	MsgContactOtherURNs     []urns.URN

	MockResponses map[string][]*httpx.MockResponse

	ExpectedRequests  []ExpectedRequest
	ExpectedExtIDs    []string
	ExpectedError     error
	ExpectedLogErrors []*svclogs.Error
	ExpectedNewURN    urns.URN
}

// Msg creates the test message for this test case
func (tc *OutgoingTestCase) Msg(ch *models.Channel) *models.MsgOut {
	msgOrigin := models.MsgOriginFlow
	if tc.MsgOrigin != "" {
		msgOrigin = tc.MsgOrigin
	}

	c := &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc", LastSeenOn: tc.MsgContactLastSeenOn, OtherURNs: tc.MsgContactOtherURNs}

	m := &models.MsgOut{
		OrgID_:                ch.OrgID(),
		UUID_:                 "0191e180-7d60-7000-aded-7d8b151cbd5b",
		Contact_:              c,
		URN_:                  urns.URN(tc.MsgURN),
		Text_:                 tc.MsgText,
		HighPriority_:         tc.MsgHighPriority,
		QuickReplies_:         tc.MsgQuickReplies,
		ResponseToExternalID_: tc.MsgResponseToExternalID,
		Origin_:               msgOrigin,
		ChannelUUID_:          ch.UUID(),
		Channel_:              ch,
	}
	m.Locale_ = tc.MsgLocale
	m.UserID_ = tc.MsgUserID
	m.Attachments_ = append(m.Attachments_, tc.MsgAttachments...)

	if tc.MsgURNAuth != "" {
		m.URNAuth_ = tc.MsgURNAuth
	}
	if tc.MsgTemplating != "" {
		templating := &models.Templating{}
		jsonx.MustUnmarshal([]byte(tc.MsgTemplating), templating)
		m.Templating_ = templating
	}
	if tc.MsgFlow != nil {
		m.Flow_ = tc.MsgFlow
	}
	return m
}

// RunOutgoingTestCases runs all the passed in test cases against the channel
func RunOutgoingTestCases(t *testing.T, channel *models.Channel, handler channels.Handler, testCases []OutgoingTestCase, checkRedacted []string, setup func(*testing.T, *runtime.Runtime)) {
	ctx, rt := testsuite.Runtime(t)

	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	// use a plain HTTP client so per-case mock transports can be installed, shared by all three clients so
	// installing a mocking transport intercepts every request a handler makes via any of them. Tracing stays
	// wrapped around whatever transport a case installs, since that's what produces the channel logs asserted below.
	client := &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Default = client
	rt.HTTP.Proxied = client
	rt.HTTP.Attachments = client

	if setup != nil {
		setup(t, rt)
	}

	s := newServer(rt)
	testsuite.InsertChannel(t, rt, channel)
	require.NoError(t, s.MountHandler(handler))

	for _, tc := range testCases {
		t.Run(tc.Label, func(t *testing.T) {
			msg := tc.Msg(channel)

			// drop any tasks a previous case queued for this contact so the assertion below sees only this case's
			rc := rt.VK.Get()
			_, err := rc.Do("DEL", contactQueueKey(channel, msg))
			rc.Close()
			require.NoError(t, err)

			var mockHTTP *httpx.MocksTransport
			actualRequests := make([]*http.Request, 0, 1)

			// reset to the default transport each case, then install a mocking transport when the
			// case provides mocks - always inside tracing, which is what the handler's channel log is built from
			rt.HTTP.Default.Transport = httpx.WithTraces(nil)
			if len(tc.MockResponses) > 0 {
				mockHTTP = httpx.WithMocks(nil, tc.MockResponses)
				rt.HTTP.Default.Transport = httpx.WithTraces(mockHTTP)
			}

			clog := models.NewChannelLogForSend(msg, handler.RedactValues(channel))
			sendCtx, cancel := context.WithTimeout(ctx, time.Millisecond*100)

			res := &channels.SendResult{}
			serr := handler.Send(sendCtx, msg, res, clog)
			externalIDs := res.ExternalIDs()

			if mockHTTP != nil {
				rt.HTTP.Default.Transport = httpx.WithTraces(nil)

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
			assert.Equal(t, append([]*svclogs.Error{}, tc.ExpectedLogErrors...), clog.Errors, "channel log errors mismatch")

			// simulate the sender completing the send so send results (e.g. new URNs) are processed
			status := models.NewStatusUpdate(channel, msg.UUID(), models.MsgStatusWired, clog)
			models.OnSendComplete(ctx, rt, msg, status, res.NewURN(), clog)

			assertContactChanged(t, rt, channel, msg, tc.ExpectedNewURN)

			AssertChannelLogRedaction(t, clog, checkRedacted)
		})
	}
}

// the key of the queue that per-contact mailroom tasks are pushed onto
func contactQueueKey(ch *models.Channel, msg *models.MsgOut) string {
	return fmt.Sprintf("c:%d:%d", ch.OrgID(), msg.Contact_.ID)
}

// asserts which contact_changed task the completed send queued: expectedNewURN is empty for the cases where none
// should be, which is what covers a send reporting a URN the contact already has.
func assertContactChanged(t *testing.T, rt *runtime.Runtime, ch *models.Channel, msg *models.MsgOut, expectedNewURN urns.URN) {
	rc := rt.VK.Get()
	defer rc.Close()

	tasks, err := redis.Strings(rc.Do("LRANGE", contactQueueKey(ch, msg), 0, -1))
	require.NoError(t, err)

	actual := make([]urns.URN, 0, len(tasks))
	for _, task := range tasks {
		payload := &struct {
			Type string `json:"type"`
			Task struct {
				NewURN struct {
					Value urns.URN `json:"value"`
				} `json:"new_urn"`
			} `json:"task"`
		}{}
		require.NoError(t, json.Unmarshal([]byte(task), payload), "error unmarshaling queued task: %s", task)

		if payload.Type == "contact_changed" {
			actual = append(actual, payload.Task.NewURN.Value)
		}
	}

	expected := make([]urns.URN, 0, 1)
	if expectedNewURN != "" {
		expected = append(expected, expectedNewURN)
	}

	assert.Equal(t, expected, actual, "contact_changed tasks mismatch")
}

// asserts that the given channel log doesn't contain any of the given values
func AssertChannelLogRedaction(t *testing.T, clog *models.ChannelLog, vals []string) {
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
