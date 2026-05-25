package runtime_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPProxied(t *testing.T) {
	// without SendProxyURL configured, HTTPProxied is the same client as HTTP
	cfg := runtime.NewDefaultConfig()
	require.NoError(t, cfg.Validate())

	rt, err := newRuntimeForHTTPTest(cfg)
	require.NoError(t, err)
	assert.Same(t, rt.HTTP, rt.HTTPProxied)

	// with SendProxyURL configured, HTTPProxied is a distinct client whose transport resolves the proxy URL
	cfg = runtime.NewDefaultConfig()
	cfg.SendProxyURL = "http://proxy.example.com:3128"
	require.NoError(t, cfg.Validate())

	rt, err = newRuntimeForHTTPTest(cfg)
	require.NoError(t, err)
	require.NotSame(t, rt.HTTP, rt.HTTPProxied)

	req, _ := http.NewRequest("POST", "https://example.org/hook", nil)
	proxy, err := rt.HTTPProxied.Transport.(*http.Transport).Proxy(req)
	require.NoError(t, err)
	require.NotNil(t, proxy, "transport should resolve a proxy URL when SendProxyURL is set")
	assert.Equal(t, "proxy.example.com:3128", proxy.Host)
	assert.Equal(t, "http", proxy.Scheme)

	// the regular HTTP client doesn't route to the configured proxy
	proxy, err = rt.HTTP.Transport.(*http.Transport).Proxy(req)
	require.NoError(t, err)
	if proxy != nil {
		assert.NotEqual(t, "proxy.example.com:3128", proxy.Host)
	}
}

// newRuntimeForHTTPTest constructs a Runtime by calling NewRuntime, which builds the HTTP and HTTPProxied
// clients we want to assert on. NewRuntime also tries to open a DB pool etc., but those don't dial
// until used, so this is safe for unit tests focused on the HTTP plumbing.
func newRuntimeForHTTPTest(cfg *runtime.Config) (*runtime.Runtime, error) {
	return runtime.NewRuntime(cfg)
}
