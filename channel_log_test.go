package courier_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestChannelLog(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.MockConnectionError,
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(1234))
	defer uuids.SetGenerator(uuids.DefaultGenerator)

	channel := test.NewMockChannel("fef91e9b-a6ed-44fb-b6ce-feed8af585a8", "NX", "1234", "US", nil)
	clog := courier.NewChannelLog(courier.ChannelLogTypeTokenFetch, channel, nil)

	// make a request that will have a response
	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.NoError(t, err)

	clog.HTTP(trace)

	// make a request that has no response (connection error)
	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err = httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	assert.EqualError(t, err, "unable to connect to server")

	clog.HTTP(trace)
	clog.Error(errors.New("this is an error"))
	clog.End()

	assert.Equal(t, uuids.UUID("c00e5d67-c275-4389-aded-7d8b151cbd5b"), clog.UUID())
	assert.Equal(t, courier.ChannelLogTypeTokenFetch, clog.Type())
	assert.Equal(t, channel, clog.Channel())
	assert.Equal(t, courier.NilMsgID, clog.MsgID())
	assert.Equal(t, 2, len(clog.HTTPLogs()))
	assert.Equal(t, 1, len(clog.Errors()))
	assert.False(t, clog.CreatedOn().IsZero())
	assert.Greater(t, clog.Elapsed(), time.Duration(0))

	hlog1 := clog.HTTPLogs()[0]
	assert.Equal(t, "https://api.messages.com/send.json", hlog1.URL)
	assert.Equal(t, 200, hlog1.StatusCode)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", hlog1.Request)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 20\r\n\r\n{\"status\":\"success\"}", hlog1.Response)

	hlog2 := clog.HTTPLogs()[1]
	assert.Equal(t, 0, hlog2.StatusCode)
	assert.Equal(t, "POST /send.json HTTP/1.1\r\nHost: api.messages.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept-Encoding: gzip\r\n\r\n", hlog2.Request)
	assert.Equal(t, "", hlog2.Response)

	err1 := clog.Errors()[0]
	assert.Equal(t, "this is an error", err1.Message())
	assert.Equal(t, "", err1.Code())

	clog.SetMsgID(courier.NewMsgID(123))
	clog.SetType(courier.ChannelLogTypeEventReceive)

	assert.Equal(t, courier.NewMsgID(123), clog.MsgID())
	assert.Equal(t, courier.ChannelLogTypeEventReceive, clog.Type())
}
