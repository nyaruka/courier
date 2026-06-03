package utils

import (
	"io"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
)

// TraceHTTP performs req, returning a trace of the request and its final response. Tracing is layered
// onto the given client's transport for this single call rather than shared on the client itself, so
// concurrent callers never race on trace state; any other middleware already on the client's transport
// (e.g. access control) stays in effect.
//
// If the server redirects, only the final hop's trace is returned — the intermediate redirect hops are
// dropped — so a redirected request yields a single trace, as httpx.DoTrace did, rather than one per
// hop. The returned trace is nil only if the request couldn't be issued at all.
//
// maxBodyBytes bounds both how many bytes are read from each response body (rejecting a larger body
// with httpx.ErrResponseSize, the protection needed when fetching from untrusted endpoints) and how
// much of the body is captured into the trace; a value <= 0 reads and captures the whole body.
//
// The returned response and error otherwise mirror http.Client.Do — on a transport error the response
// is nil and the error is set — except that a body-read error (e.g. ErrResponseSize), which the tracing
// transport defers onto the response body, is surfaced here as the returned error so callers see it the
// same way httpx.DoTrace reported it. The body remains available via the trace's ResponseBody.
func TraceHTTP(client *http.Client, req *http.Request, maxBodyBytes int) (*httpx.Trace, *http.Response, error) {
	// WithReadLimit (inside WithTraces, so the bound applies before the body is buffered) caps the
	// bytes read from the response body; WithTraces then captures that bounded body into the trace.
	tracing := httpx.WithTraces(httpx.WithReadLimit(client.Transport, maxBodyBytes))

	resp, err := (&http.Client{
		Transport:     tracing,
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}).Do(req)

	// When a read limit is in effect, WithReadLimit surfaces an oversized body as a read error which
	// WithTraces replays on resp.Body rather than returning. Drain the final response to surface that
	// (or any other deferred read error) as the returned error, as httpx.DoTrace did; the body is still
	// available via the trace's ResponseBody.
	if err == nil && maxBodyBytes > 0 && resp != nil {
		if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
			err = drainErr
		}
	}

	// keep only the final hop's trace; on a redirect the earlier hops are the 3xx responses that led
	// here and would otherwise each produce a separate channel-log entry
	traces := tracing.Traces()
	if len(traces) == 0 {
		return nil, resp, err
	}
	return traces[len(traces)-1], resp, err
}
