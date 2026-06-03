package utils_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripFunc adapts a function to an http.RoundTripper for tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTraceHTTP(t *testing.T) {
	const url = "https://example.com/thing"

	clientWithBody := func(body []byte) *http.Client {
		return &http.Client{Transport: httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
			url: {httpx.NewMockResponse(200, nil, body)},
		})}
	}

	// a body within the limit is read in full, captured into the trace, and returns no error
	req, _ := http.NewRequest("GET", url, nil)
	trace, resp, err := utils.TraceHTTP(clientWithBody([]byte("hello")), req, 1024)
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte("hello"), trace.ResponseBody)

	// a body exceeding the limit is surfaced as ErrResponseSize (deferred onto the body by the tracing
	// transport, then drained back out by TraceHTTP)
	req, _ = http.NewRequest("GET", url, nil)
	_, _, err = utils.TraceHTTP(clientWithBody(bytes.Repeat([]byte("x"), 100)), req, 10)
	assert.ErrorIs(t, err, httpx.ErrResponseSize)

	// a limit of 0 disables the bound: the whole body is read and captured with no error
	req, _ = http.NewRequest("GET", url, nil)
	trace, _, err = utils.TraceHTTP(clientWithBody(bytes.Repeat([]byte("x"), 100)), req, 0)
	require.NoError(t, err)
	assert.Len(t, trace.ResponseBody, 100)

	// a redirect yields only the final hop's trace, not one per hop
	redirectClient := &http.Client{Transport: httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"https://example.com/redirect": {httpx.NewMockResponse(302, map[string]string{"Location": url}, nil)},
		url:                            {httpx.NewMockResponse(200, nil, []byte("final"))},
	})}
	req, _ = http.NewRequest("GET", "https://example.com/redirect", nil)
	trace, resp, err = utils.TraceHTTP(redirectClient, req, 0)
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []byte("final"), trace.ResponseBody)

	// a body-read error must be surfaced as the returned error even with no read limit (maxBodyBytes
	// 0), matching httpx.DoTrace — WithTraces only replays it on resp.Body, so TraceHTTP must drain
	// unconditionally to recover it
	errClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := io.MultiReader(bytes.NewReader([]byte("partial")), iotest.ErrReader(io.ErrUnexpectedEOF))
		return &http.Response{Status: "200 OK", StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: make(http.Header), Body: io.NopCloser(body)}, nil
	})}
	req, _ = http.NewRequest("GET", url, nil)
	_, _, err = utils.TraceHTTP(errClient, req, 0)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}
