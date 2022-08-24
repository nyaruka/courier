package courier_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
)

func TestNewChannelLogFromTrace(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.MockConnectionError,
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	channel := test.NewMockChannel("fef91e9b-a6ed-44fb-b6ce-feed8af585a8", "NX", "1234", "US", nil)

	// make a request that will have a response
	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)

	log := courier.NewLegacyChannelLog("Send message", channel, courier.NewMsgID(1234), trace)

	assert.Equal(t, "Send message", log.Description)
	assert.Equal(t, channel, log.Channel)
	assert.Equal(t, courier.NewMsgID(1234), log.MsgID)
	assert.Equal(t, "POST", log.Method)
	assert.Equal(t, "https://api.messages.com/send.json", log.URL)
	assert.Equal(t, 200, log.StatusCode)
	assert.Equal(t, "", log.Error)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", log.Request)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 20\r\n\r\n{\"status\":\"success\"}", log.Response)
	assert.Equal(t, trace.StartTime, log.CreatedOn)
	assert.Equal(t, trace.EndTime.Sub(trace.StartTime), log.Elapsed)

	// make a request that has no response (connection error)
	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.EqualError(t, err, "unable to connect to server")

	log = courier.NewLegacyChannelLog("Send message", channel, courier.NewMsgID(1234), trace)

	assert.Equal(t, 0, log.StatusCode)
	assert.Equal(t, "", log.Error)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", log.Request)
	assert.Equal(t, "", log.Response)
}
