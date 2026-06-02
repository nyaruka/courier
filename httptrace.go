package courier

import (
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
)

// TraceHTTP performs req, capturing an HTTP trace of each request and response. Tracing is layered onto
// the given client's transport for this single call rather than shared on the client itself, so
// concurrent callers never race on trace state; any other middleware already on the client's transport
// (e.g. access control) stays in effect. Each request hop is captured as its own trace — one in the
// common case, or several if the server issues redirects. At most maxBodyBytes of each response body
// are captured into its trace (a value <= 0 captures the whole body), while the response body remains
// readable via the returned response. The returned response and error are exactly those of
// http.Client.Do, so on a transport error the response is nil and the error is set.
//
// Note that, unlike httpx.DoTrace with a positive limit, this reads each response body fully into
// memory; callers that must bound the bytes read from an untrusted endpoint should use httpx.DoTrace.
func TraceHTTP(client *http.Client, req *http.Request, maxBodyBytes int) ([]*httpx.Trace, *http.Response, error) {
	tracing := httpx.WithTracing(client.Transport, maxBodyBytes)

	resp, err := (&http.Client{
		Transport:     tracing,
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}).Do(req)

	return tracing.Traces(), resp, err
}
