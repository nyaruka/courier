package chatbase

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
)

func TestChatbase(t *testing.T) {
	var testRequest *http.Request
	var statusCode = 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		testRequest = httptest.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(body))
		testRequest.Header = r.Header
		w.WriteHeader(statusCode)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	chatbaseAPIURL = server.URL

	now := time.Now()
	err := SendChatbaseMessage("apiKey", "apiVersion", "messageType", "userID", "platform", "message", now)
	assert.NoError(t, err)

	// parse our body
	bytes, err := ioutil.ReadAll(testRequest.Body)
	assert.NoError(t, err)

	// check our request body
	str, err := jsonparser.GetString(bytes, "type")
	assert.NoError(t, err)
	assert.Equal(t, "messageType", str)

	str, err = jsonparser.GetString(bytes, "version")
	assert.NoError(t, err)
	assert.Equal(t, "apiVersion", str)

	ts, err := jsonparser.GetInt(bytes, "time_stamp")
	assert.NoError(t, err)
	assert.Equal(t, now.UnixNano()/int64(time.Millisecond), ts)

	// simulate an error
	statusCode = 500
	err = SendChatbaseMessage("apiKey", "apiVersion", "messageType", "userID", "platform", "message", now)
	assert.Error(t, err)
}
