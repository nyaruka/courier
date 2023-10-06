package courier_test

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	logger := slog.Default()
	config := courier.NewConfig()
	config.StatusUsername = "admin"
	config.StatusPassword = "password123"

	mb := test.NewMockBackend()
	mb.AddChannel(test.NewMockChannel("95710b36-855d-4832-a723-5f71f73688a0", "MCK", "12345", "RW", nil))

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
	statusCode, respBody := request("GET", "http://localhost:8080/", "", "")
	assert.Equal(t, 200, statusCode)
	assert.Contains(t, respBody, "/c/mck/{uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}}/receive - Mock Handler receive")

	// can't access status page without auth
	statusCode, respBody = request("GET", "http://localhost:8080/status", "", "")
	assert.Equal(t, 401, statusCode)
	assert.Contains(t, respBody, "Unauthorized")

	// can access status page without auth
	statusCode, respBody = request("GET", "http://localhost:8080/status", "admin", "password123")
	assert.Equal(t, 200, statusCode)
	assert.Contains(t, respBody, "ALL GOOD")

	// can't access status page with wrong method
	statusCode, respBody = request("POST", "http://localhost:8080/status", "admin", "password123")
	assert.Equal(t, 405, statusCode)
	assert.Contains(t, respBody, "Method Not Allowed")

	// can't access non-existent page
	statusCode, respBody = request("POST", "http://localhost:8080/nothere", "admin", "password123")
	assert.Equal(t, 404, statusCode)
	assert.Contains(t, respBody, "not found")
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
	uuids.SetGenerator(uuids.NewSeededGenerator(1234))

	logger := slog.Default()
	config := courier.NewConfig()
	config.AuthToken = "sesame"

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]any{})
	mb.AddChannel(mockChannel)

	server := courier.NewServerWithLogger(config, mb, logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8080/c/_fetch-attachment", strings.NewReader(body))
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
	assert.JSONEq(t, `{"attachment": {"content_type": "image/jpeg", "url": "https://backend.com/attachments/cdf7ed27-5ad5-4028-b664-880fc7581c77.jpg", "size": 17301}, "log_uuid": "c00e5d67-c275-4389-aded-7d8b151cbd5b"}`, string(respBody))

	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal(t, courier.ChannelLogTypeAttachmentFetch, clog.Type())
	assert.Len(t, clog.HTTPLogs(), 1)
	assert.Greater(t, clog.Elapsed(), time.Duration(0))

	// if fetching attachment from channel returns non-200, return unavailable attachment so caller doesn't retry
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.mp3"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.mp3", "size": 0}, "log_uuid": "547deaf7-7620-4434-95b3-58675999c4b7"}`, string(respBody))

	// same if fetching attachment times out
	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/hello.pdf"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"attachment": {"content_type": "unavailable", "url": "http://mock.com/media/hello.pdf", "size": 0}, "log_uuid": "338ff339-5663-49ed-8ef6-384876655d1b"}`, string(respBody))
}
