package runtime

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/vkutil"
	"github.com/vinovest/sqlx"
)

// MaxAttachmentBodyBytes bounds how much of an incoming attachment we'll read. Attachments are fetched from URLs
// chosen by the remote service rather than by us, so the bound is what stops one from being able to make us buffer
// an arbitrarily large body.
const MaxAttachmentBodyBytes = 100 * 1024 * 1024

type Runtime struct {
	Config     *Config
	DB         *sqlx.DB
	Dynamo     *Dynamo
	VK         *redis.Pool
	S3         *s3x.Service
	CW         *cwatch.Service
	Centrifugo *centrifugo.Service

	// HTTP is the general purpose client for outbound calls. Its transport captures a trace of any request whose
	// context carries an httpx.TraceCollector, which is how callers get the trace to attach to a channel log.
	HTTP *http.Client

	// HTTPProxied is the HTTP client used by handlers that send to user-configured URLs. When
	// SendProxyURL is set, it routes through that forward proxy. Otherwise it's the same as HTTP.
	HTTPProxied *http.Client

	// HTTPAttachments is the client used to fetch incoming attachments. It is the only path on which we fetch from
	// a URL chosen by the remote service rather than by us, so it bounds how much of a response body it will read;
	// reading past that fails with httpx.ErrResponseSize. The bound sits inside the tracing so it applies before the
	// body is buffered into the trace.
	HTTPAttachments *http.Client

	Stats *StatsCollector
}

func NewRuntime(cfg *Config) (*Runtime, error) {
	rt := &Runtime{Config: cfg}

	var err error

	rt.DB, err = sqlx.Open("postgres", cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("error creating Postgres connection pool: %w", err)
	}
	rt.DB.SetMaxIdleConns(4)
	rt.DB.SetMaxOpenConns(16)

	ctx := context.Background()

	// the AWS service constructors resolve credentials and region from the SDK default chain (env
	// vars, instance/task IAM role, shared config/credentials files, etc.)
	rt.Dynamo, err = newDynamo(ctx, cfg)
	if err != nil {
		return nil, err
	}

	rt.VK, err = vkutil.NewPool(cfg.Valkey, vkutil.WithMaxActive(cfg.MaxWorkers*2))
	if err != nil {
		return nil, fmt.Errorf("error creating Valkey pool: %w", err)
	}

	rt.S3, err = s3x.NewService(ctx, cfg.S3Endpoint, cfg.S3PathStyle)
	if err != nil {
		return nil, fmt.Errorf("error creating S3 service: %w", err)
	}

	rt.CW, err = cwatch.NewService(ctx, cfg.CloudwatchNamespace, cfg.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("error creating Cloudwatch service: %w", err)
	}

	rt.Centrifugo = centrifugo.NewService(centrifugo.NewClient(cfg.CentrifugoEndpoint, cfg.CentrifugoKey), rt.VK)

	// parse the SSRF blocklist up front so it can be baked into each HTTP client's transport via
	// httpx.WithAccessControl, rather than passed to every request.
	disallowedIPs, disallowedNets, err := cfg.ParseDisallowedNetworks()
	if err != nil {
		return nil, fmt.Errorf("error parsing disallowed networks: %w", err)
	}
	httpAccess := httpx.NewAccessConfig(10*time.Second, disallowedIPs, disallowedNets)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 64
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 15 * time.Second

	// tracing goes on the outside of each client so that a request denied by access control is still captured as a
	// trace. It records into whichever httpx.TraceCollector the request's context carries, and costs nothing for a
	// request made without one, so it belongs here rather than being assembled per call.
	rt.HTTP = &http.Client{Transport: httpx.WithTraces(httpx.WithAccessControl(transport, httpAccess)), Timeout: 30 * time.Second}

	// the attachments client additionally bounds the response body, since it fetches from URLs we didn't choose
	rt.HTTPAttachments = &http.Client{
		Transport: httpx.WithTraces(httpx.WithReadLimit(httpx.WithAccessControl(transport, httpAccess), MaxAttachmentBodyBytes)),
		Timeout:   30 * time.Second,
	}

	// build a proxied variant when SendProxyURL is configured; otherwise reuse the regular client
	// so handlers can always go through HTTPProxied without behavior change.
	//
	// Note on the SSRF blocklist: the access control wrapped into each transport resolves the
	// destination URL's host and rejects the request if it maps to a disallowed IP. This check runs
	// regardless of the proxy; when the proxy is set the request still dials the proxy rather than the
	// destination, so the proxy's own egress rules govern the actual connection to the destination.
	proxyURL, err := cfg.ParseSendProxyURL()
	if err != nil {
		return nil, fmt.Errorf("error parsing send proxy URL: %w", err)
	}
	rt.HTTPProxied = rt.HTTP
	if proxyURL != nil {
		proxiedTransport := transport.Clone()
		proxiedTransport.Proxy = http.ProxyURL(proxyURL)
		rt.HTTPProxied = &http.Client{Transport: httpx.WithTraces(httpx.WithAccessControl(proxiedTransport, httpAccess)), Timeout: 30 * time.Second}
	}

	rt.Stats = NewStatsCollector()

	return rt, nil
}

// NewTestRuntime returns a minimal Runtime wrapping the given config, suitable for tests that need a
// Runtime but don't bring up real backing services. It populates HTTP (shared by HTTPProxied) with a
// dedicated client so code paths that issue outbound HTTP requests work against test servers, and so
// tests can install a mocking transport via httpx.WithMocks without mutating http.DefaultClient.
func NewTestRuntime(cfg *Config) *Runtime {
	// give the client a timeout matching the production clients so a test that accidentally lets a
	// request escape its mocking transport fails fast instead of hanging, and the same tracing the
	// production clients carry so code under test produces channel logs the way it does in production
	client := &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	return &Runtime{
		Config:          cfg,
		HTTP:            client,
		HTTPProxied:     client,
		HTTPAttachments: client,
		// note the nil valkey pool: publishing requires a subscriber presence lookup, so tests that
		// exercise a publish path need a runtime with a real pool (i.e. testsuite.Runtime)
		Centrifugo: centrifugo.NewService(centrifugo.NewMockClient(), nil),
		Stats:      NewStatsCollector(),
	}
}

func (r *Runtime) Start() error {
	return r.Dynamo.start()
}

func (r *Runtime) Stop() {
	r.Dynamo.stop()
}
