package runtime

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	awsx "github.com/nyaruka/gocommon/aws"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/vkutil"
	"github.com/vinovest/sqlx"
)

type Runtime struct {
	Config *Config
	DB     *sqlx.DB
	Dynamo *dynamodb.Client
	VK     *redis.Pool
	S3     *s3x.Service
	CW     *cwatch.Service

	// AWSRegion is the region resolved from the standard AWS SDK default chain. It's kept here so code
	// that needs to reason about region-qualified S3 hostnames (e.g. media URL resolution) can use it
	// without a courier-specific region config setting.
	AWSRegion string

	HTTP *http.Client

	// HTTPProxied is the HTTP client used by handlers that send to user-configured URLs. When
	// SendProxyURL is set, it routes through that forward proxy. Otherwise it's the same as HTTP.
	HTTPProxied *http.Client

	Writers *Writers
	Spool   *dynamo.Spool
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

	// resolve the AWS region from the standard SDK default chain (AWS_REGION / AWS_DEFAULT_REGION env
	// vars, shared config, etc.) so we can reason about region-qualified S3 hostnames without a
	// courier-specific region setting.
	awsCfg, err := awsx.NewConfig("", "", "")
	if err != nil {
		return nil, fmt.Errorf("error resolving AWS config: %w", err)
	}
	rt.AWSRegion = awsCfg.Region

	// pass empty credentials and region to the AWS service constructors so the SDK resolves them from
	// its default chain (env vars, instance/task IAM role, shared config/credentials files, etc.)
	rt.Dynamo, err = dynamo.NewClient("", "", "", cfg.DynamoEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error creating DynamoDB client: %w", err)
	}

	rt.VK, err = vkutil.NewPool(cfg.Valkey, vkutil.WithMaxActive(cfg.MaxWorkers*2))
	if err != nil {
		return nil, fmt.Errorf("error creating Valkey pool: %w", err)
	}

	rt.S3, err = s3x.NewService("", "", "", cfg.S3Endpoint, cfg.S3PathStyle)
	if err != nil {
		return nil, fmt.Errorf("error creating S3 service: %w", err)
	}

	rt.CW, err = cwatch.NewService("", "", "", cfg.CloudwatchNamespace, cfg.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("error creating Cloudwatch service: %w", err)
	}

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
	rt.HTTP = &http.Client{Transport: httpx.WithAccessControl(transport, httpAccess), Timeout: 30 * time.Second}

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
		rt.HTTPProxied = &http.Client{Transport: httpx.WithAccessControl(proxiedTransport, httpAccess), Timeout: 30 * time.Second}
	}

	rt.Spool = dynamo.NewSpool(rt.Dynamo, rt.Config.SpoolDir+"/dynamo", 30*time.Second)
	rt.Writers = newWriters(cfg, rt.Dynamo, rt.Spool)

	return rt, nil
}

// NewTestRuntime returns a minimal Runtime wrapping the given config, suitable for tests that need a
// Runtime but don't bring up real backing services. It populates HTTP (shared by HTTPProxied) with a
// dedicated client so code paths that issue outbound HTTP requests work against test servers, and so
// tests can install a mocking transport via httpx.WithMocks without mutating http.DefaultClient.
func NewTestRuntime(cfg *Config) *Runtime {
	// give the client a timeout matching the production clients so a test that accidentally lets a
	// request escape its mocking transport fails fast instead of hanging
	client := &http.Client{Timeout: 30 * time.Second}
	return &Runtime{Config: cfg, HTTP: client, HTTPProxied: client}
}

func (r *Runtime) Start() error {
	if err := r.Spool.Start(); err != nil {
		return fmt.Errorf("error starting dynamo spool: %w", err)
	}

	r.Writers.start()
	return nil
}

func (r *Runtime) Stop() {
	r.Writers.stop()
	r.Spool.Stop()
}
