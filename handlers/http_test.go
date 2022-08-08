package handlers_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
)

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
