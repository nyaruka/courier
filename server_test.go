package courier_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	logger := logrus.New()
	config := courier.NewConfig()
	config.StatusUsername = "admin"
	config.StatusPassword = "password123"

	server := courier.NewServerWithLogger(config, test.NewMockBackend(), logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	// hit our main pages, this is admitedly mostly in the name of coverage
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
	trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)
	assert.Contains(t, string(trace.ResponseBody), "courier")

	// status page without auth
	req, _ = http.NewRequest("GET", "http://localhost:8080/status", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)
	assert.Equal(t, 401, trace.Response.StatusCode)

	// status page with auth
	req, _ = http.NewRequest("GET", "http://localhost:8080/status", nil)
	req.SetBasicAuth("admin", "password123")
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)
	assert.Contains(t, string(trace.ResponseBody), "courier")

	// hit an invalid path
	req, _ = http.NewRequest("GET", "http://localhost:8080/notthere", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)
	assert.Contains(t, string(trace.ResponseBody), "not found")

	// invalid method
	req, _ = http.NewRequest("POST", "http://localhost:8080/", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)
	assert.Contains(t, string(trace.ResponseBody), "method not allowed")
}

func TestFetchAttachment(t *testing.T) {
	testJPG := test.ReadFile("test/testdata/test.jpg")

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://mock.com/media/hello.jpg": {
			httpx.NewMockResponse(200, nil, testJPG),
		},
	}))

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234))

	logger := logrus.New()
	config := courier.NewConfig()
	config.StatusUsername = "admin"
	config.StatusPassword = "password123"

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", map[string]interface{}{})
	mb.AddChannel(mockChannel)

	server := courier.NewServerWithLogger(config, mb, logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	submit := func(body string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8080/fetch-attachment", strings.NewReader(body))
		fmt.Println(req.Host)
		fmt.Println(req.URL.Hostname())
		trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, trace.ResponseBody
	}

	// try to submit with empty body
	statusCode, respBody := submit(`{}`)
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'ChannelType' failed on the 'required' tag`)

	// try to submit with non-existent channel
	statusCode, respBody = submit(`{"channel_uuid": "c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "channel_type": "VV", "url": "http://mock.com/media/test.jpg"}`)
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel not found`)

	statusCode, respBody = submit(`{"channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "url": "http://mock.com/media/test.jpg"}`)
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"log_uuid":"c00e5d67-c275-4389-aded-7d8b151cbd5b", "size": 15238, "url": "https://backend.com/attachments/cdf7ed27-5ad5-4028-b664-880fc7581c77.jpg"}`, string(respBody))
}
