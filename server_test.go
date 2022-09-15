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
