package test

import (
	"net/http"
	"os"

	"github.com/nyaruka/gocommon/httpx"
)

func ReadFile(path string) []byte {
	d, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return d
}

// DoTraced makes the given request through the given client - whose transport must be wrapped with httpx.WithTraces -
// and returns the trace it captured alongside any error. It's the test equivalent of what handlers do via
// BaseHandler.RequestHTTP: put a collector on the request context, make the call, take the trace back off.
func DoTraced(client *http.Client, req *http.Request) (*httpx.Trace, error) {
	ctx, traces := httpx.WithTraceCollector(req.Context())
	resp, err := client.Do(req.WithContext(ctx))
	if resp != nil {
		resp.Body.Close()
	}
	return traces.Last(), err
}

// MockTransport returns a transport serving the given mocks, wrapped in tracing. Tests install it on a runtime
// client's Transport in place of the real stack. The tracing must stay in the stack: it's what captures the traces
// that handlers attach to their channel logs, so a bare mocking transport would leave every log empty.
func MockTransport(mocks map[string][]*httpx.MockResponse) http.RoundTripper {
	return httpx.WithTraces(httpx.WithMocks(nil, mocks))
}
