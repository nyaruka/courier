package courier

import (
	"io"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
)

// TraceHTTP performs req, capturing an HTTP trace of each request and response. Tracing is layered onto
// the given client's transport for this single call rather than shared on the client itself, so
// concurrent callers never race on trace state; any other middleware already on the client's transport
// (e.g. access control) stays in effect. Each request hop is captured as its own trace — one in the
// common case, or several if the server issues redirects, with the captured trace and the returned
// response being those of the final hop.
//
// maxBodyBytes bounds both how many bytes are read from each response body (rejecting a larger body
// with httpx.ErrResponseSize, the protection needed when fetching from untrusted endpoints) and how
// much of the body is captured into the trace; a value <= 0 reads and captures the whole body.
//
// The returned response and error otherwise mirror http.Client.Do — on a transport error the response
// is nil and the error is set — except that a body-read error (e.g. ErrResponseSize), which the tracing
// transport defers onto the response body, is surfaced here as the returned error so callers see it the
// same way httpx.DoTrace reported it. The body remains available via the final trace's ResponseBody.
func TraceHTTP(client *http.Client, req *http.Request, maxBodyBytes int) ([]*httpx.Trace, *http.Response, error) {
	// WithBodyLimit (inside WithTracing, so the bound applies before the body is buffered) caps the
	// bytes read; WithTracing then captures up to maxBodyBytes of that bounded body into the trace.
	tracing := httpx.WithTracing(httpx.WithBodyLimit(client.Transport, maxBodyBytes), maxBodyBytes)

	resp, err := (&http.Client{
		Transport:     tracing,
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}).Do(req)

	// When a body limit is in effect, WithBodyLimit surfaces an oversized body as a read error which
	// WithTracing replays on resp.Body rather than returning. Drain the final response to surface that
	// (or any other deferred read error) as the returned error, as httpx.DoTrace did; the body is still
	// available via the trace's ResponseBody.
	if err == nil && maxBodyBytes > 0 && resp != nil {
		if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
			err = drainErr
		}
	}

	return tracing.Traces(), resp, err
}
