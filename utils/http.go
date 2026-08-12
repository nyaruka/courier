package utils

import (
	"io"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
)

// DoTraced performs req through the given client and returns the trace of that single call alongside the response.
// The client's transport must be wrapped with httpx.WithTraces, as the runtime's clients are; a collector is put on
// the request context so that only this call's trace comes back even though the client is shared.
//
// The response body is drained before returning. The tracing transport buffers the body into the trace and defers any
// read error onto the handed-back body rather than returning it, so a caller which takes its bytes from the trace -
// which is what all of ours do - would otherwise accept a body truncated by a read limit, or a short read, as though
// it were complete. Draining here surfaces that as the returned error instead, and is the reason to go through this
// rather than calling client.Do directly.
func DoTraced(client *http.Client, req *http.Request) (*httpx.Trace, *http.Response, error) {
	ctx, traces := httpx.WithTraceCollector(req.Context())

	resp, err := client.Do(req.WithContext(ctx))

	if err == nil && resp != nil {
		if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
			err = drainErr
		}
	}

	return traces.Last(), resp, err
}
