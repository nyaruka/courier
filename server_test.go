package courier_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	logger := logrus.New()
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
