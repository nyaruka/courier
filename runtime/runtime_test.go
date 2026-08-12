package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPProxied(t *testing.T) {
	// the default config uses virtual-host style S3 URLs, which require a resolvable region
	t.Setenv("AWS_REGION", "us-east-1")

	// without SendProxyURL configured, HTTP.Proxied is the same client as HTTP.Default
	cfg := runtime.NewDefaultConfig()
	require.NoError(t, cfg.Parse())

	rt, err := newRuntimeForHTTPTest(cfg)
	require.NoError(t, err)
	assert.Same(t, rt.HTTP.Default, rt.HTTP.Proxied)

	// the blocklist Config.Parse built is baked into the client's transport, so a request to a disallowed address
	// is rejected without dialing. This is the assertion that fails if a runtime is ever built from a config that
	// wasn't parsed, since an unparsed config yields an empty blocklist rather than an error.
	_, err = rt.HTTP.Default.Get("http://127.0.0.1:1/")
	assert.ErrorIs(t, err, httpx.ErrAccessConfig)

	// stand up a stub forward proxy; a forward-proxied http:// request arrives here carrying the
	// target's host, which lets us confirm the proxied client actually routes through it. The access
	// control wrapped onto the transport hides the underlying *http.Transport, so we assert on
	// behavior rather than inspecting the proxy func directly.
	var proxiedHost string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	// with SendProxyURL configured, HTTP.Proxied is a distinct client that routes through the proxy.
	// clear the SSRF blocklist so the IP-literal target below (never actually dialed) isn't rejected
	// by access control before it can be proxied.
	cfg = runtime.NewDefaultConfig()
	cfg.DisallowedNetworks = nil
	cfg.SendProxyURL = proxy.URL
	require.NoError(t, cfg.Parse())

	rt, err = newRuntimeForHTTPTest(cfg)
	require.NoError(t, err)
	require.NotSame(t, rt.HTTP.Default, rt.HTTP.Proxied)

	resp, err := rt.HTTP.Proxied.Get("http://93.184.216.34/hook")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "93.184.216.34", proxiedHost, "proxied client should route through the configured proxy")
}

// newRuntimeForHTTPTest constructs a Runtime by calling NewRuntime, which builds the HTTP clients we want
// to assert on. NewRuntime also tries to open a DB pool etc., but those don't dial
// until used, so this is safe for unit tests focused on the HTTP plumbing.
func newRuntimeForHTTPTest(cfg *runtime.Config) (*runtime.Runtime, error) {
	return runtime.NewRuntime(cfg)
}
