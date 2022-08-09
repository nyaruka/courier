package handlers_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func TestDoHTTPRequest(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"status":"error"}`)),
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	mb := courier.NewMockBackend()
	mc := courier.NewMockChannel("7a8ff1d4-f211-4492-9d05-e1905f6da8c8", "NX", "1234", "EC", nil)
	mm := mb.NewOutgoingMsg(mc, courier.NewMsgID(123), urns.URN("tel:+1234"), "Hello World", false, nil, "", "")
	logger := courier.NewChannelLoggerForSend(mm)

	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, respBody, err := handlers.RequestHTTP(req, logger)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte(`{"status":"success"}`), respBody)
	assert.Len(t, logger.Logs(), 1)

	log1 := logger.Logs()[0]
	assert.Equal(t, 200, log1.StatusCode)
	assert.Equal(t, mc, log1.Channel)
	assert.Equal(t, courier.NewMsgID(123), log1.MsgID)
	assert.Equal(t, "https://api.messages.com/send.json", log1.URL)

	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, respBody, err = handlers.RequestHTTP(req, logger)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Len(t, logger.Logs(), 2)

	log2 := logger.Logs()[1]
	assert.Equal(t, 400, log2.StatusCode)
	assert.Equal(t, mc, log2.Channel)
	assert.Equal(t, courier.NewMsgID(123), log2.MsgID)
	assert.Equal(t, "https://api.messages.com/send.json", log2.URL)
}

func TestMakeHTTPRequest(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"status":"error"}`)),
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err := handlers.MakeHTTPRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, trace.Response.StatusCode)
	assert.Equal(t, []byte(`{"status":"success"}`), trace.ResponseBody)

	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err = handlers.MakeHTTPRequest(req)
	assert.EqualError(t, err, "received non 200 status: 400")
	assert.Equal(t, 400, trace.Response.StatusCode)
	assert.Equal(t, []byte(`{"status":"error"}`), trace.ResponseBody)
}
