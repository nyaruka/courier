package librato

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
)

func TestLibrato(t *testing.T) {
	var testRequest *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		testRequest = httptest.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(body))
		testRequest.Header = r.Header
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	libratoEndpoint = server.URL

	// create a new sender
	wg := sync.WaitGroup{}
	sender := NewSender(&wg, "username", "password", "host", 10*time.Millisecond)
	sender.Start()

	// queue up some events
	sender.AddGauge("event10", 10)
	sender.AddGauge("event11", 11)
	sender.AddGauge("event12", 12)

	// sleep a bit
	time.Sleep(20 * time.Millisecond)

	// our server should have been called, check the parameters
	assert.NotNil(t, testRequest)
	assert.Equal(t, "POST", testRequest.Method)

	body, _ := ioutil.ReadAll(testRequest.Body)

	source, err := jsonparser.GetString(body, "source")
	assert.NoError(t, err)
	assert.Equal(t, "host", source)

	gauge10, err := jsonparser.GetString(body, "gauges", "[0]", "name")
	assert.NoError(t, err)
	assert.Equal(t, "event10", gauge10)

	gauge12, err := jsonparser.GetInt(body, "gauges", "[2]", "value")
	assert.NoError(t, err)
	assert.Equal(t, int64(12), gauge12)

	sender.Stop()
	wg.Wait()
}
