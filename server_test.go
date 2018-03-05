package courier

import (
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	logger := logrus.New()
	config := NewConfig()
	config.StatusUsername = "admin"
	config.StatusPassword = "password123"

	server := NewServerWithLogger(config, NewMockBackend(), logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	// hit our main pages, this is admitedly mostly in the name of coverage
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
	rr, err := utils.MakeHTTPRequest(req)
	assert.NoError(t, err)
	assert.Contains(t, string(rr.Body), "courier")

	// status page without auth
	req, _ = http.NewRequest("GET", "http://localhost:8080/status", nil)
	rr, err = utils.MakeHTTPRequest(req)
	assert.Error(t, err)
	assert.Equal(t, 401, rr.StatusCode)

	// status page with auth
	req, _ = http.NewRequest("GET", "http://localhost:8080/status", nil)
	req.SetBasicAuth("admin", "password123")
	rr, err = utils.MakeHTTPRequest(req)
	assert.NoError(t, err)
	assert.Contains(t, string(rr.Body), "courier")

	// hit an invalid path
	req, _ = http.NewRequest("GET", "http://localhost:8080/notthere", nil)
	rr, err = utils.MakeHTTPRequest(req)
	assert.Error(t, err)
	assert.Contains(t, string(rr.Body), "not found")

	// invalid method
	req, _ = http.NewRequest("POST", "http://localhost:8080/", nil)
	rr, err = utils.MakeHTTPRequest(req)
	assert.Error(t, err)
	assert.Contains(t, string(rr.Body), "method not allowed")
}
