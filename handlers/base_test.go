package handlers_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func TestRequestHTTP(t *testing.T) {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"status":"error"}`)),
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	mb := test.NewMockBackend()
	mc := test.NewMockChannel("7a8ff1d4-f211-4492-9d05-e1905f6da8c8", "NX", "1234", "EC", nil)
	mm := mb.NewOutgoingMsg(mc, 123, urns.URN("tel:+1234"), "Hello World", false, nil, "", "", courier.MsgOriginChat, nil)
	clog := courier.NewChannelLogForSend(mm, nil)

	config := courier.NewConfig()
	server := test.NewMockServer(config, mb)

	h := handlers.NewBaseHandler("NX", "Test")
	h.SetServer(server)

	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, respBody, err := h.RequestHTTP(req, clog)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte(`{"status":"success"}`), respBody)
	assert.Len(t, clog.HTTPLogs(), 1)

	hlog1 := clog.HTTPLogs()[0]
	assert.Equal(t, 200, hlog1.StatusCode)
	assert.Equal(t, "https://api.messages.com/send.json", hlog1.URL)

	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, _, err = h.RequestHTTP(req, clog)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Len(t, clog.HTTPLogs(), 2)

	hlog2 := clog.HTTPLogs()[1]
	assert.Equal(t, 400, hlog2.StatusCode)
	assert.Equal(t, "https://api.messages.com/send.json", hlog2.URL)
}
