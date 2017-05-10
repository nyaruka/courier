package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fmt"

	_ "github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/stretchr/testify/require"
)

type RequestPrepFunc func(*http.Request)

type ChannelTestCase struct {
	Label string

	URL      string
	Data     string
	Status   int
	Response string

	Name      *string
	Text      *string
	URN       *string
	External  *string
	MediaURL  *string
	MediaURLs []string
	Date      *time.Time

	PrepRequest RequestPrepFunc
}

func (c ChannelTestCase) GetMediaURLs() []string {
	if c.MediaURL != nil {
		return []string{*c.MediaURL}
	}
	return c.MediaURLs
}

// utility method to get the pointer to the passed in string
func Sp(str string) *string { return &str }

// utility method to get the pointer to the pased in time
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
func testHandlerRequest(tb testing.TB, s *courier.MockServer, url string, data string, expectedStatus int, expectedBody *string, requestPrepFunc RequestPrepFunc) string {
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

func RunChannelTestCases(t *testing.T, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelTestCase) {
	s := courier.NewMockServer()
	for _, ch := range channels {
		s.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		t.Run(testCase.Label, func(t *testing.T) {
			require := require.New(t)

			s.ClearQueueMsgs()

			testHandlerRequest(t, s, testCase.URL, testCase.Data, testCase.Status, &testCase.Response, testCase.PrepRequest)

			// pop our message off and test against it
			msg, err := s.GetLastQueueMsg()

			if testCase.Status == 200 && testCase.Text != nil {
				require.Nil(err)

				defer msg.Release()

				if testCase.Name != nil {
					require.Equal(*testCase.Name, msg.Name())
				}
				if testCase.Text != nil {
					require.Equal(*testCase.Text, msg.Text())
				}
				if testCase.URN != nil {
					require.Equal(*testCase.URN, string(msg.URN()))
				}
				if testCase.External != nil {
					require.Equal(*testCase.External, msg.ExternalID())
				}
				if testCase.MediaURL != nil || len(testCase.MediaURLs) > 0 {
					require.Equal(testCase.GetMediaURLs(), msg.MediaURLs())
				}
				if testCase.Date != nil {
					require.Equal(*testCase.Date, msg.Date())
				}
			} else if err != courier.ErrNoMsg {
				t.Fatalf("unexpected msg inserted: %v", err)
			}
		})
	}

	// check non-channel specific error conditions against first test case
	validCase := testCases[0]

	t.Run("Queue Error", func(t *testing.T) {
		s.SetErrorOnQueue(true)
		defer s.SetErrorOnQueue(false)
		testHandlerRequest(t, s, validCase.URL, validCase.Data, 400, Sp("unable to queue message"), validCase.PrepRequest)
	})

	t.Run("Receive With Invalid Channel", func(t *testing.T) {
		s.ClearChannels()
		testHandlerRequest(t, s, validCase.URL, validCase.Data, 400, Sp("channel not found"), validCase.PrepRequest)
	})
}

func RunChannelBenchmarks(b *testing.B, channels []courier.Channel, handler courier.ChannelHandler, testCases []ChannelTestCase) {
	s := courier.NewMockServer()
	for _, ch := range channels {
		s.AddChannel(ch)
	}
	handler.Initialize(s)

	for _, testCase := range testCases {
		s.ClearQueueMsgs()

		b.Run(testCase.Label, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				testHandlerRequest(b, s, testCase.URL, testCase.Data, testCase.Status, nil, testCase.PrepRequest)
			}
		})
	}
}
