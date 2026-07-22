package courier_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendEvent(t *testing.T) {
	_, rt := testsuite.Runtime(t) // throttling needs a real valkey
	rt.Config.AuthToken = "sesame"
	rt.Config.InternetPort = 8180
	rt.Config.InternalPort = 8181

	testsuite.ResetValkey(t, rt)

	mb := test.NewMockBackend()
	mockChannel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(mockChannel)

	// add a channel that also supports typing stopped events
	stoppableChannel := test.NewMockChannel("f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{"supports_stop": true})
	mb.AddChannel(stoppableChannel)

	// add a channel whose type has no handler registered and thus can't send events
	brokenChannel := test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{})
	mb.AddChannel(brokenChannel)

	server := courier.NewServer(rt, mb)
	server.Runtime().HTTP.Transport = httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/action": {
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
			httpx.NewMockResponse(200, nil, []byte(`OK`)),
		},
	})
	require.NoError(t, server.Start())
	defer server.Stop()

	submit := func(body, authToken string) (int, []byte) {
		req, _ := http.NewRequest("POST", "http://localhost:8181/ci/event/send", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		trace, _, err := utils.TraceHTTP(http.DefaultClient, req, 0)
		require.NoError(t, err)
		return trace.Response.StatusCode, trace.ResponseBody
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
	assert.Len(t, mb.WrittenChannelLogs(), 0) // and no channel log is written

	// submit for a channel that can
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))

	// successful sends don't write channel logs
	assert.Len(t, mb.WrittenChannelLogs(), 0)

	// repeating within the interval for the same conversation is throttled - reported as success but no
	// send is made (the mock transport's next response isn't consumed)
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123123"), "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"supported": true, "interval": 10}`, string(respBody))
	assert.Len(t, mb.WrittenChannelLogs(), 0)

	// a send error (different URN so not throttled) returns an error response and writes a channel log
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123124"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel connection failed`)

	assert.Len(t, mb.WrittenChannelLogs(), 1)
	clog := mb.WrittenChannelLogs()[0]
	assert.Equal(t, courier.ChannelLogTypeEventSend, clog.Type)
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "http://mock.com/action", clog.HttpLogs[0].URL)

	// and clears the throttle, so a retry for that conversation attempts another send (consuming the
	// second 502 mock) instead of being suppressed as a success
	statusCode, respBody = submit(typingEvent("MCK", "typing_started", "outgoing", "e4bb1578-29da-4fa5-a214-9da19dd24230", "tel:+250788123124"), "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, string(respBody), `channel connection failed`)
	assert.Len(t, mb.WrittenChannelLogs(), 2)

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
	assert.Len(t, mb.WrittenChannelLogs(), 2) // and all of that succeeded so no new channel logs
}
