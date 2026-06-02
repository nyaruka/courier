package courier_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceHTTP(t *testing.T) {
	const url = "https://example.com/thing"

	clientWithBody := func(body []byte) *http.Client {
		return &http.Client{Transport: httpx.WithMocking(nil, map[string][]*httpx.MockResponse{
			url: {httpx.NewMockResponse(200, nil, body)},
		})}
	}

	// a body within the limit is read in full, captured into the trace, and returns no error
	req, _ := http.NewRequest("GET", url, nil)
	traces, resp, err := courier.TraceHTTP(clientWithBody([]byte("hello")), req, 1024)
	require.NoError(t, err)
	require.Len(t, traces, 1)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte("hello"), traces[0].ResponseBody)

	// a body exceeding the limit is surfaced as ErrResponseSize (deferred onto the body by the tracing
	// transport, then drained back out by TraceHTTP)
	req, _ = http.NewRequest("GET", url, nil)
	_, _, err = courier.TraceHTTP(clientWithBody(bytes.Repeat([]byte("x"), 100)), req, 10)
	assert.ErrorIs(t, err, httpx.ErrResponseSize)

	// a limit of 0 disables the bound: the whole body is read and captured with no error
	req, _ = http.NewRequest("GET", url, nil)
	traces, _, err = courier.TraceHTTP(clientWithBody(bytes.Repeat([]byte("x"), 100)), req, 0)
	require.NoError(t, err)
	assert.Len(t, traces[0].ResponseBody, 100)
}
