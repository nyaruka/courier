package web_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// counts the event send channel logs written to DynamoDB
func countChannelLogs(t *testing.T, rt *runtime.Runtime) int {
	count := 0
	for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
		if strings.HasPrefix(item.Key.SK, "log#") && item.Data["type"] == "event_send" {
			count++
		}
	}
	return count
}

// asserts that the number of channel logs written to DynamoDB reaches the expected count (writes are async)
func assertChannelLogCount(t *testing.T, rt *runtime.Runtime, expected int) {
	if expected == 0 {
		time.Sleep(400 * time.Millisecond) // give the writer time to write anything pending
		assert.Equal(t, 0, countChannelLogs(t, rt))
	} else {
		require.Eventually(t, func() bool { return countChannelLogs(t, rt) == expected }, 5*time.Second, 100*time.Millisecond)
	}
}

func TestSendEvent(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	rt.Config.AuthToken = "sesame"
	rt.Config.InternetPort = 8180
	rt.Config.InternalPort = 8181

	server := web.NewServer(rt)
	server.Runtime().HTTP.Default.Transport = httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/action": {
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
		},
	}))
	require.NoError(t, server.Start())
	defer server.Stop()

	testsuite.InsertChannel(t, rt, test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	// add a channel that also supports typing stopped events
	testsuite.InsertChannel(t, rt, test.NewMockChannel("f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{"supports_stop": true}))

	// add a channel whose type has no handler registered and thus can't send events
	testsuite.InsertChannel(t, rt, test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8181/ci/event/send", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, respBody
	}

	// builds a request body with a typing event routed to the given channel type/uuid/urn
	typingEvent := func(channelType, eventType, direction, channelUUID, urn string) string {
		event := `{"uuid": "0197b335-6ded-79a4-95a6-3af85b57f108", "type": "` + eventType + `", "created_on": "2026-07-15T12:00:00Z", "direction": "` + direction + `"`
		if channelUUID != "" {
			event += `, "channel": {"uuid": "` + channelUUID + `", "name": "Test"}`
		}
		if urn != "" {
			event += `, "urn": "` + urn + `"`
		}
		event += `}`
		return `{"channel_type": "` + channelType + `", "event": ` + event + `}`
	}

	// try to submit with no auth header
	statusCode, respBody := submit(`{}`, "")
	assert.Equal(t, 401, statusCode)
	assert.Equal(t, "Unauthorized", string(respBody))

	// try to submit with empty body
	statusCode, respBody = submit(`{}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `Field validation for 'ChannelType' failed on the 'required' tag`)
	assert.Contains(t, string(respBody), `Field validation for 'Event' failed on the 'required' tag`)

	// try to submit with an event type that isn't registered
	statusCode, respBody = submit(typingEvent("MCK", "dancing", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `unknown type: 'dancing'`)

	// try to submit with a real event type that isn't sendable
	statusCode, respBody = submit(`{"channel_type": "MCK", "event": {"uuid": "0197b335-6ded-79a4-95a6-3af85b57f108", "type": "contact_language_changed", "created_on": "2026-07-15T12:00:00Z", "language": "eng"}}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `contact_language_changed is not a sendable event type`)

	// try to submit an incoming event - only user/bot originated events can be sent to a platform
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "incoming", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `only outgoing events can be sent`)

	// try to submit an event missing the channel or urn routing fields
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "", "tel:+250788123123"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `event requires channel and urn to be sent`)
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", ""), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `event requires channel and urn to be sent`)

	// a sendable event type the channel's handler doesn't declare support for isn't an error but isn't supported
	statusCode, respBody = submit(typingEvent("MCK", "typing_stopped", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": false}`, string(respBody))

	// try to submit with non-existent channel
	statusCode, respBody = submit(typingEvent("VV", "typing_started", "outgoing", "c25aab53-f23a-46c9-8ae3-1af850ad9fd9", "tel:+250788123123"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel not found`)

	// submitting for a channel type that can't send events isn't an error but response says unsupported
	statusCode, respBody = submit(typingEvent("XX", "typing_started", "outgoing", "53e5aafa-8155-449d-9009-fcb30d54bd26", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": false}`, string(respBody))
	assertChannelLogCount(t, rt, 0) // and no channel log is written

	// submit for a channel that can
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))

	// successful sends don't write channel logs
	assertChannelLogCount(t, rt, 0)

	// repeating within the interval for the same conversation is throttled - reported as success but no
	// send is made (the mock transport's next response isn't consumed)
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))
	assertChannelLogCount(t, rt, 0)

	// a send error (different URN so not throttled) returns an error response and writes a channel log
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123124"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel connection failed`)

	assertChannelLogCount(t, rt, 1)

	// check the content of the written channel log
	for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
		if strings.HasPrefix(item.Key.SK, "log#") {
			assert.Equal(t, "event_send", item.Data["type"])

			var dataGZ map[string]any
			require.NoError(t, dynamo.UnmarshalJSONGZ(item.DataGZ, &dataGZ))
			httpLogs := dataGZ["http_logs"].([]any)
			assert.Len(t, httpLogs, 1)
			assert.Equal(t, "http://mock.com/action", httpLogs[0].(map[string]any)["url"])
		}
	}

	// and clears the throttle, so a retry for that conversation attempts another send (consuming the
	// second 502 mock) instead of being suppressed as a success
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123124"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel connection failed`)
	assertChannelLogCount(t, rt, 2)

	// a typing started on the stoppable channel is sent and throttled as usual
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))

	// a typing stopped is a one-shot send (no interval in response) which ends the typing session...
	statusCode, respBody = submit(typingEvent("MCK", "typing_stopped", "outgoing", "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true}`, string(respBody))

	// ...clearing the started throttle so a new typing session isn't suppressed - this send consumes the
	// last mock response, which repeating within the interval wouldn't
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))
	assertChannelLogCount(t, rt, 2) // and all of that succeeded so no new channel logs
}
