package courier_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() *courier.Config {
	config := courier.NewDefaultConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Valkey = "valkey://localhost:6379/0"
	config.Port = 8081
	return config
}

func TestServerURLs(t *testing.T) {
	logger := slog.Default()
	config := testConfig()
	config.StatusUsername = "admin"
	config.StatusPassword = "password123"

	mb := test.NewMockBackend()
	mb.AddChannel(test.NewMockChannel("95710b36-855d-4832-a723-5f71f73688a0", "MCK", "12345", "RW", []string{urns.Phone.Prefix}, nil))

	server := courier.NewServerWithLogger(config, mb, logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	request := func(method, url, user, pass string) (int, string) {
		req, _ := http.NewRequest(method, url, nil)
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, string(trace.ResponseBody)
	}

	// route listing at the / root
	statusCode, respBody := request("GET", "http://localhost:8081/", "", "")
	assert.Equal(t, 200, statusCode)
	assert.Contains(t, respBody, "/c/mck/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}/receive - Mock Handler receive")

	// can't access status page without auth
	statusCode, respBody = request("GET", "http://localhost:8081/status", "", "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, respBody, "Unauthorized")

	// can access status page without auth
	statusCode, respBody = request("GET", "http://localhost:8081/status", "admin", "password123")
	assert.Equal(t, 200, statusCode)
	assert.Contains(t, respBody, "ALL GOOD")

	// can't access status page with wrong method
	statusCode, respBody = request("POST", "http://localhost:8081/status", "admin", "password123")
	assert.Equal(t, 405, statusCode)
	assert.Equal(t, respBody, "{\"message\":\"Method Not Allowed\",\"data\":[{\"type\":\"error\",\"error\":\"method not allowed: POST\"}]}\n")

	// can't access non-existent page
	statusCode, respBody = request("POST", "http://localhost:8081/nothere", "admin", "password123")
	assert.Equal(t, 404, statusCode)
	assert.Equal(t, respBody, "{\"message\":\"Not Found\",\"data\":[{\"type\":\"error\",\"error\":\"not found: /nothere\"}]}\n")
}

func TestIncoming(t *testing.T) {
	// create and start our backend and server
	mb := test.NewMockBackend()
	s := courier.NewServer(testConfig(), mb)

	s.Start()
	defer s.Stop()

	resp, err := http.Get("http://localhost:8081/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive")
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "missing from or text")

	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Len(t, clog.HttpLogs, 1)

	req, _ := http.NewRequest("GET", "http://localhost:8081/c/mck/e4bb1578-29da-4fa5-a214-9da19dd24230/receive?from=2065551212&text=hello", nil)
	req.Header.Set("Cookie", "secret")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ok")

	assert.Len(t, mb.WrittenChannelLogs(), 2)
	clog = mb.WrittenChannelLogs()[1]
	assert.Len(t, clog.HttpLogs, 1)
}

func TestOutgoing(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://mock.com/send": {
			httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, []byte(`SENT`)),
			httpx.NewMockResponse(429, nil, []byte(`too much!`)),
			httpx.NewMockResponse(403, nil, []byte(`stop!`)),
		},
	}))

	// create and start our backend and server
	mb := test.NewMockBackend()
	s := courier.NewServer(testConfig(), mb)

	s.Start()
	defer s.Stop()

	// create two channels but only register one of them
	brokenChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	// try to send message via unregistered channel
	msg := test.NewMockMsg(courier.MsgID(101), courier.NilMsgUUID, brokenChannel, "tel:+250788383383", "test message", nil)
	sendAndWait(mb, msg)

	// message should have failed...
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, msg.ID(), mb.WrittenMsgStatuses()[0].MsgID())
	assert.Equal(t, courier.MsgStatusFailed, mb.WrittenMsgStatuses()[0].Status())
	assert.Equal(t, 1, len(mb.WrittenChannelLogs()))
	mb.Reset()

	// send message via registered channel
	msg = test.NewMockMsg(courier.MsgID(102), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "test message 2", nil)
	sendAndWait(mb, msg)

	// message should be marked as wired
	assert.Len(t, mb.WrittenMsgStatuses(), 1)
	status := mb.WrittenMsgStatuses()[0]
	assert.Equal(t, msg.ID(), status.MsgID())
	assert.Equal(t, courier.MsgStatusWired, status.Status())

	// and we should have a channel log with redacted errors and traces
	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal(t, []*clogs.Error{{Code: "seeds", Message: "contains ********** seeds"}}, clog.Errors)
	assert.Len(t, clog.HttpLogs, 1)

	hlog := clog.HttpLogs[0]
	assert.Equal(t, "http://mock.com/send", hlog.URL)
	assert.Equal(t,
		"GET /send HTTP/1.1\r\nHost: mock.com\r\nUser-Agent: Go-http-client/1.1\r\nAuthorization: Token **********\r\nAccept-Encoding: gzip\r\n\r\n",
		hlog.Request,
	)
	mb.Reset()

	// send the message again, should be skipped but again marked as wired
	mb.PushOutgoingMsg(msg)
	time.Sleep(time.Millisecond * 500)

	// message should be marked as wired
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, msg.ID(), mb.WrittenMsgStatuses()[0].MsgID())
	assert.Equal(t, courier.MsgStatusWired, mb.WrittenMsgStatuses()[0].Status())
	mb.Reset()

	// send message which will have mocked connection error
	sendAndWait(mb, test.NewMockMsg(courier.MsgID(103), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "3", nil))

	// message should be marked as errored (retryable)
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, courier.MsgStatusErrored, mb.WrittenMsgStatuses()[0].Status())
	mb.Reset()

	// send message which will have mocked channel config error
	sendAndWait(mb, test.NewMockMsg(courier.MsgID(104), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "err:config", nil))

	// message should be marked as failed (non-retryable)
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, courier.MsgStatusFailed, mb.WrittenMsgStatuses()[0].Status())
	mb.Reset()

	// send message which will have mocked rate limiting error
	sendAndWait(mb, test.NewMockMsg(courier.MsgID(105), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "5", nil))

	// message should be marked as errored (retryable)
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, courier.MsgStatusErrored, mb.WrittenMsgStatuses()[0].Status())
	mb.Reset()

	// send message which will have mocked contact-stopped error
	sendAndWait(mb, test.NewMockMsg(courier.MsgID(106), courier.NilMsgUUID, mockChannel, "tel:+250788383383", "6", nil))

	// message should be marked as failed (non-retryable)
	assert.Equal(t, 1, len(mb.WrittenMsgStatuses()))
	assert.Equal(t, courier.MsgStatusFailed, mb.WrittenMsgStatuses()[0].Status())

	// and we should have created a contact stop event
	assert.Equal(t, 1, len(mb.WrittenChannelEvents()))
	assert.Equal(t, courier.EventTypeStopContact, mb.WrittenChannelEvents()[0].EventType())
	mb.Reset()
}

func TestFetchAttachment(t *testing.T) {
	testJPG := test.ReadFile("test/testdata/test.jpg")

	httpMocks := httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://mock.com/media/hello.jpg": {
			httpx.NewMockResponse(200, nil, testJPG),
		},
		"http://mock.com/media/hello.mp3": {
			httpx.NewMockResponse(404, nil, []byte(`No such file`)),
		},
		"http://mock.com/media/hello.pdf": {
			httpx.MockConnectionError,
		},
	})
	httpMocks.SetIgnoreLocal(true)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpMocks)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, dates.NewSequentialNow(time.Date(2024, 9, 11, 14, 33, 0, 0, time.UTC), time.Second)))

	logger := slog.Default()
	config := courier.NewDefaultConfig()
	config.AuthToken = "sesame"
	config.Port = 8081

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	server := courier.NewServerWithLogger(config, mb, logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8081/c/_fetch-attachment", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, trace.ResponseBody
	}

	// try to submit with no auth header
	statusCode, respBody := submit(`{}`, "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with wrong auth header
	statusCode, respBody = submit(`{}`, "23462")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with empty body
	statusCode, respBody = submit(`{}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'ChannelType' failed on the 'required' tag`)

	// try to submit with non-existent channel
	statusCode, respBody = submit(`{"channel_uuid": "c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "channel_type": "VV", "url": "http://mock.com/media/hello.jpg"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel not found`)

	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.jpg"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "image/jpeg", "url": "https://backend.com/attachments/cdf7ed27-5ad5-4028-b664-880fc7581c77.jpg", "size": 17301}, "log_uuid": "0191e180-7d60-7000-aded-7d8b151cbd5b"}`, string(respBody))

	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal(t, courier.ChannelLogTypeAttachmentFetch, clog.Type)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Greater(t, clog.Elapsed, time.Duration(0))

	// if fetching attachment from channel returns non-200, return unavailable attachment so caller doesn't retry
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.mp3"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.mp3", "size": 0}, "log_uuid": "0191e180-8148-7000-95b3-58675999c4b7"}`, string(respBody))

	// same if fetching attachment times out
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.pdf"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.pdf", "size": 0}, "log_uuid": "0191e180-8530-7000-8ef6-384876655d1b"}`, string(respBody))
}

// utility to send a message on a mocked backend and block until it's marked as sent
func sendAndWait(mb *test.MockBackend, m courier.MsgOut) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	mb.PushOutgoingMsg(m)

	for {
		time.Sleep(time.Millisecond * 25)

		if sent, _ := mb.WasMsgSent(ctx, m.ID()); sent {
			return
		}
	}
}
