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

// MockTransport returns a transport serving the given mocks, wrapped in tracing. Tests install it on a runtime
// client's Transport in place of the real stack. The tracing must stay in the stack: it's what captures the traces
// that handlers attach to their channel logs, so a bare mocking transport would leave every log empty.
func MockTransport(mocks map[string][]*httpx.MockResponse) http.RoundTripper {
	return httpx.WithTraces(httpx.WithMocks(nil, mocks))
}
