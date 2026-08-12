package runtime

import (
	"net/http"
	"time"

	"github.com/nyaruka/gocommon/httpx"
)

// MaxAttachmentBodyBytes bounds how much of an incoming attachment we'll read. Attachments are fetched from URLs
// chosen by the remote service rather than by us, so the bound is what stops one from being able to make us buffer
// an arbitrarily large body.
const MaxAttachmentBodyBytes = 100 * 1024 * 1024

// HTTP holds the http.Clients used for outbound calls. Default is the general purpose one. Proxied is for handlers
// sending to user-configured URLs, which route through the forward proxy when one is configured. Attachments is for
// fetching incoming attachments, the only path on which we fetch from a URL chosen by the remote service rather than
// by us, and so the only one that bounds how much of a response body it will read.
//
// Every client's transport captures a trace of any request whose context carries an httpx.TraceCollector, which is
// how callers get the trace to attach to a channel log. Tests can replace a client's Transport with a mocking
// transport to intercept its calls - that must be one which keeps the tracing, see test.MockTransport.
type HTTP struct {
	Default     *http.Client
	Proxied     *http.Client
	Attachments *http.Client
}

func newHTTP(cfg *Config) *HTTP {
	// the SSRF blocklist is baked into each client's transport via httpx.WithAccessControl, rather than passed to
	// every request. Config.Parse turned the configured networks into the IPs and nets this needs.
	access := httpx.NewAccessConfig(10*time.Second, cfg.DisallowedIPs, cfg.DisallowedNets)

	transport := newBaseTransport()

	// tracing goes on the outside of each client so that a request denied by access control is still captured as a
	// trace. It records into whichever httpx.TraceCollector the request's context carries, and costs nothing for a
	// request made without one, so it belongs here rather than being assembled per call.
	h := &HTTP{
		Default: &http.Client{Transport: httpx.WithTraces(httpx.WithAccessControl(transport, access)), Timeout: 30 * time.Second},

		// the attachments client additionally bounds the response body, since it fetches from URLs we didn't
		// choose. The bound sits inside the tracing so it applies before the body is buffered into the trace;
		// reading past it fails with httpx.ErrResponseSize.
		Attachments: &http.Client{
			Transport: httpx.WithTraces(httpx.WithReadLimit(httpx.WithAccessControl(transport, access), MaxAttachmentBodyBytes)),
			Timeout:   30 * time.Second,
		},
	}

	// build a proxied variant when SendProxyURL is configured; otherwise reuse the default client so handlers can
	// always go through Proxied without behavior change.
	//
	// Note on the SSRF blocklist: the access control wrapped into each transport resolves the destination URL's
	// host and rejects the request if it maps to a disallowed IP. This check runs regardless of the proxy; when the
	// proxy is set the request still dials the proxy rather than the destination, so the proxy's own egress rules
	// govern the actual connection to the destination.
	h.Proxied = h.Default
	if cfg.SendProxyURLParsed != nil {
		proxiedTransport := transport.Clone()
		proxiedTransport.Proxy = http.ProxyURL(cfg.SendProxyURLParsed)
		h.Proxied = &http.Client{Transport: httpx.WithTraces(httpx.WithAccessControl(proxiedTransport, access)), Timeout: 30 * time.Second}
	}

	return h
}

// newTestHTTP builds the clients for a test runtime, which don't need access control or the proxy. All three are the
// same client: they're given a timeout matching the production clients so a test that accidentally lets a request
// escape its mocking transport fails fast instead of hanging, and the same tracing the production clients carry so
// code under test produces channel logs the way it does in production.
func newTestHTTP() *HTTP {
	client := &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}

	return &HTTP{Default: client, Proxied: client, Attachments: client}
}

// newBaseTransport clones http.DefaultTransport with our connection-pool settings. Callers layer their own concerns
// (access control, the send proxy, read limits) on top.
func newBaseTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 64
	t.MaxIdleConnsPerHost = 8
	t.IdleConnTimeout = 15 * time.Second
	return t
}
