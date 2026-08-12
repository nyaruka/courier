package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func TestRequestHTTP(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	mc := test.NewMockChannel("7a8ff1d4-f211-4492-9d05-e1905f6da8c8", "NX", "1234", "EC", []string{urns.Phone.Prefix}, nil)
	cf := &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"}
	mm := &models.MsgOut{
		OrgID_:       mc.OrgID(),
		UUID_:        "019a06fa-467d-7fc8-a11e-3ad2d019fd20",
		Contact_:     cf,
		URN_:         urns.URN("tel:+1234"),
		Text_:        "Hello World",
		Origin_:      models.MsgOriginChat,
		ChannelUUID_: mc.UUID(),
		Channel_:     mc,
	}
	clog := models.NewChannelLogForSend(mm, nil)

	// use a plain client so we can install a mocking transport
	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default
	rt.HTTP.Default.Transport = httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
			httpx.NewMockResponse(400, nil, []byte(`{"status":"error"}`)),
		},
	}))

	server := web.NewServer(rt)

	h := handlers.NewBaseHandler("NX", "Test")
	h.SetRuntime(server.Runtime())

	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, respBody, err := h.RequestHTTP(req, clog)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte(`{"status":"success"}`), respBody)
	assert.Len(t, clog.HttpLogs, 1)

	hlog1 := clog.HttpLogs[0]
	assert.Equal(t, 200, hlog1.StatusCode)
	assert.Equal(t, "https://api.messages.com/send.json", hlog1.URL)

	req, _ = http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	resp, _, err = h.RequestHTTP(req, clog)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Len(t, clog.HttpLogs, 2)

	hlog2 := clog.HttpLogs[1]
	assert.Equal(t, 400, hlog2.StatusCode)
	assert.Equal(t, "https://api.messages.com/send.json", hlog2.URL)
}
