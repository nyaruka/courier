package courier_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendChatAction(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.AuthToken = "sesame"
	cfg.InternetPort = 8180
	cfg.InternalPort = 8181

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	// add a channel whose type has no handler registered and thus can't send chat actions
	brokenChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(brokenChannel)

	server := courier.NewServer(runtime.NewTestRuntime(cfg), mb)
	server.Runtime().HTTP.Transport = httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/action": {
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
		},
	})
	require.NoError(t, server.Start())
	defer server.Stop()

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8181/ci/chat_action/send", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		trace, _, err := utils.TraceHTTP(http.DefaultClient, req, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, trace.ResponseBody
	}

	// try to submit with no auth header
	statusCode, respBody := submit(`{}`, "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with empty body
	statusCode, respBody = submit(`{}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'Action' failed on the 'required' tag`)

	// try to submit with an invalid action
	statusCode, respBody = submit(`{"action": "dancing", "channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'Action' failed on the 'oneof' tag`)

	// a valid action the channel's handler doesn't declare support for isn't an error but isn't supported
	statusCode, respBody = submit(`{"action": "typing_stopped", "channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": false}`, string(respBody))

	// try to submit with non-existent channel
	statusCode, respBody = submit(`{"action": "typing_started", "channel_uuid": "c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "channel_type": "VV", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel not found`)

	// submitting for a channel type that can't send chat actions isn't an error but response says unsupported
	statusCode, respBody = submit(`{"action": "typing_started", "channel_uuid": "53e5aafa-8155-449d-9009-fcb30d54bd26", "channel_type": "XX", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": false}`, string(respBody))
	assert.Len(t, mb.WrittenChannelLogs(), 0) // and no channel log is written

	// submit for a channel that can
	statusCode, respBody = submit(`{"action": "typing_started", "channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))

	// successful sends don't write channel logs
	assert.Len(t, mb.WrittenChannelLogs(), 0)

	// a send error returns an error response and writes a channel log
	statusCode, respBody = submit(`{"action": "typing_started", "channel_uuid": "e4bb1578-29da-4fa5-a214-9da19dd24230", "channel_type": "MCK", "urn": "tel:+250788123123"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel connection failed`)

	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal(t, courier.ChannelLogTypeChatActionSend, clog.Type)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "http://mock.com/action", clog.HttpLogs[0].URL)
}

func TestChannelInfo(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.AuthToken = "sesame"
	cfg.InternetPort = 8180
	cfg.InternalPort = 8181

	mb := test.NewMockBackend()
	mb.AddChannel(test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))
	mb.AddChannel(test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	server := courier.NewServer(runtime.NewTestRuntime(cfg), mb)
	require.NoError(t, server.Start())
	defer server.Stop()

	fetch := func(path, authToken string) (int, []byte) {
		req, _ := http.NewRequest("GET", "http://localhost:8181/ci/channel/info/"+path, nil)
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		trace, _, err := utils.TraceHTTP(http.DefaultClient, req, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, trace.ResponseBody
	}

	// no auth
	statusCode, respBody := fetch("e4bb1578-29da-4fa5-a214-9da19dd24230", "")
	assert.Equal(t, 401, statusCode)

	// missing or invalid uuid doesn't match the route
	statusCode, _ = fetch("", "sesame")
	assert.Equal(t, 404, statusCode)
	statusCode, _ = fetch("notauuid", "sesame")
	assert.Equal(t, 404, statusCode)

	// non-existent channel
	statusCode, respBody = fetch("c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), "channel not found")

	// channel whose handler declares chat action support
	statusCode, respBody = fetch("e4bb1578-29da-4fa5-a214-9da19dd24230", "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"chat_actions": {"typing_started": 10}}`, string(respBody))

	// channel with no handler has no capabilities to declare
	statusCode, respBody = fetch("53e5aafa-8155-449d-9009-fcb30d54bd26", "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{}`, string(respBody))
}
