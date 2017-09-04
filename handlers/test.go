package handlers

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fmt"

	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// RequestPrepFunc is our type for a hook for tests to use before a request is fired in a test
type RequestPrepFunc func(*http.Request)

// ChannelHandleTestCase defines the test values for a particular test case
type ChannelHandleTestCase struct {
	Label string

	URL      string
	Data     string
	Status   int
	Response string

	Name         *string
	Text         *string
	URN          *string
	External     *string
	Attachment   *string
	Attachments  []string
	Date         *time.Time
	ChannelEvent *string

	PrepRequest RequestPrepFunc
}

// SendPrepFunc allows test cases to modify the channel, msg or server before a message is sent
type SendPrepFunc func(*httptest.Server, courier.Channel, courier.Msg)

// ChannelSendTestCase defines the test values for a particular test case
type ChannelSendTestCase struct {
	Label string

	Text        string
	URN         string
	Attachments []string
	Priority    courier.MsgPriority

	ResponseStatus int
	ResponseBody   string

	URLParams   map[string]string
	PostParams  map[string]string
	RequestBody string
	Headers     map[string]string

	Error      string
	Status     string
	ExternalID string

	Stopped bool

	SendPrep SendPrepFunc
}

// Sp is a utility method to get the pointer to the passed in string
func Sp(str string) *string { return &str }

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
func testHandlerRequest(tb testing.TB, s courier.Server, url string, data string, expectedStatus int, expectedBody *string, requestPrepFunc RequestPrepFunc) string {
	var req *http.Request
	var err error

	if data != "" {
		req, err = http.NewRequest("POST", url, strings.NewReader(data))

		// guess our content type
		contentType := "application/x-www-form-urlencoded"
		if strings.Contains(data, "{") && strings.Contains(data, "}") {
			contentType = "application/json"
		} else if strings.Contains(data, "<") && strings.Contains(data, ">") {
			contentType = "application/xml"
		}
		req.Header.Set("Content-Type", contentType)
	} else {
		req, err = http.NewRequest("GET", url, nil)
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

	return courier.NewServerWithLogger(config.NewTest(), backend, logger)
}

// RunChannelSendTestCases runs all the passed in test cases against the channel
func RunChannelSendTestCases(t *testing.T, channel courier.Channel, handler courier.ChannelHandler, testCases []ChannelSendTestCase) {
	mb := courier.NewMockBackend()
	s := newServer(mb)
	mb.AddChannel(channel)
	handler.Initialize(s)

	for _, testCase := range testCases {
		t.Run(testCase.Label, func(t *testing.T) {
			require := require.New(t)

			priority := courier.DefaultPriority
			if testCase.Priority != 0 {
				priority = testCase.Priority
			}
			msg := mb.NewOutgoingMsg(channel, courier.NewMsgID(10), courier.URN(testCase.URN), testCase.Text, priority)
			for _, a := range testCase.Attachments {
				msg.WithAttachment(a)
			}

			var testRequest *http.Request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := ioutil.ReadAll(r.Body)
				testRequest = httptest.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(body))
				testRequest.Header = r.Header
				w.WriteHeader(testCase.ResponseStatus)
				w.Write([]byte(testCase.ResponseBody))
			}))
			defer server.Close()

			// call our prep function if we have one
			if testCase.SendPrep != nil {
				testCase.SendPrep(server, channel, msg)
			}

			status, err := handler.SendMsg(msg)

			if testCase.Error != "" {
				if err == nil {
					t.Errorf("expected error: %s", testCase.Error)
				} else {
					require.Equal(testCase.Error, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %s", err.Error())
			}

			require.NotNil(testRequest)

			if testCase.URLParams != nil {
				for k, v := range testCase.URLParams {
					value := testRequest.URL.Query().Get(k)
					require.Equal(v, value, fmt.Sprintf("%s not equal", k))
				}
			}

			if testCase.PostParams != nil {
				for k, v := range testCase.PostParams {
					value := testRequest.PostFormValue(k)
					require.Equal(v, value)
				}
			}

			if testCase.RequestBody != "" {
				value, _ := ioutil.ReadAll(testRequest.Body)
				require.Equal(testCase.RequestBody, string(value))
			}

			if testCase.Headers != nil {
				for k, v := range testCase.Headers {
					value := testRequest.Header.Get(k)
					require.Equal(v, value)
				}
			}

			if testCase.ExternalID != "" {
				require.Equal(testCase.ExternalID, status.ExternalID())
			}

			if testCase.Status != "" {
				require.Equal(testCase.Status, string(status.Status()))
			}

			if testCase.Stopped {
				require.Equal(msg, mb.GetLastStoppedMsgContact())
			}
		})
	}

}

// RunChannelTestCases runs all the passed in tests cases for the passed in channel configurations
func RunChannelTestCases(t *testing.T, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelHandleTestCase) {
	mb := courier.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		t.Run(testCase.Label, func(t *testing.T) {
			require := require.New(t)

			mb.ClearQueueMsgs()

			testHandlerRequest(t, s, testCase.URL, testCase.Data, testCase.Status, &testCase.Response, testCase.PrepRequest)

			// pop our message off and test against it
			contactName := mb.GetLastContactName()
			msg, _ := mb.GetLastQueueMsg()
			event, _ := mb.GetLastChannelEvent()

			if testCase.Status == 200 {
				if testCase.Name != nil {
					require.Equal(*testCase.Name, contactName)
				}
				if testCase.Text != nil {
					require.Equal(*testCase.Text, msg.Text())
				}
				if testCase.ChannelEvent != nil {
					require.Equal(*testCase.ChannelEvent, string(event.EventType()))
				}
				if testCase.URN != nil {
					if msg != nil {
						require.Equal(*testCase.URN, string(msg.URN()))
					} else if event != nil {
						require.Equal(*testCase.URN, string(event.URN()))
					} else {
						require.Equal(*testCase.URN, "")
					}
				}
				if testCase.External != nil {
					require.Equal(*testCase.External, msg.ExternalID())
				}
				if testCase.Attachment != nil {
					require.Equal([]string{*testCase.Attachment}, msg.Attachments())
				}
				if len(testCase.Attachments) > 0 {
					require.Equal(testCase.Attachments, msg.Attachments())
				}
				if testCase.Date != nil {
					if msg != nil {
						require.Equal(*testCase.Date, *msg.ReceivedOn())
					} else if event != nil {
						require.Equal(*testCase.Date, event.OccurredOn())
					} else {
						require.Equal(*testCase.Date, nil)
					}
				}
			}
		})
	}

	// check non-channel specific error conditions against first test case
	validCase := testCases[0]

	t.Run("Queue Error", func(t *testing.T) {
		mb.SetErrorOnQueue(true)
		defer mb.SetErrorOnQueue(false)
		testHandlerRequest(t, s, validCase.URL, validCase.Data, 400, Sp("unable to queue message"), validCase.PrepRequest)
	})

	t.Run("Receive With Invalid Channel", func(t *testing.T) {
		mb.ClearChannels()
		testHandlerRequest(t, s, validCase.URL, validCase.Data, 400, Sp("channel not found"), validCase.PrepRequest)
	})
}

// RunChannelBenchmarks runs all the passed in test cases for the passed in channels
func RunChannelBenchmarks(b *testing.B, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelHandleTestCase) {
	mb := courier.NewMockBackend()
	s := newServer(mb)

	for _, ch := range channels {
		mb.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		mb.ClearQueueMsgs()

		b.Run(testCase.Label, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				testHandlerRequest(b, s, testCase.URL, testCase.Data, testCase.Status, nil, testCase.PrepRequest)
			}
		})
	}
}
