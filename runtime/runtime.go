package runtime

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
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

	HTTP       *http.Client
	HTTPAccess *httpx.AccessConfig

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

	rt.Dynamo, err = dynamo.NewClient(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.DynamoEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error creating DynamoDB client: %w", err)
	}

	rt.VK, err = vkutil.NewPool(cfg.Valkey, vkutil.WithMaxActive(cfg.MaxWorkers*2))
	if err != nil {
		return nil, fmt.Errorf("error creating Valkey pool: %w", err)
	}

	rt.S3, err = s3x.NewService(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.S3Endpoint, cfg.S3PathStyle)
	if err != nil {
		return nil, fmt.Errorf("error creating S3 service: %w", err)
	}

	rt.CW, err = cwatch.NewService(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.CloudwatchNamespace, cfg.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("error creating Cloudwatch service: %w", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 64
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 15 * time.Second
	rt.HTTP = &http.Client{Transport: transport, Timeout: 30 * time.Second}

	// build a proxied variant when SendProxyURL is configured; otherwise reuse the regular client
	// so handlers can always go through HTTPProxied without behavior change
	rt.HTTPProxied = rt.HTTP
	if cfg.SendProxyURLParsed != nil {
		proxiedTransport := transport.Clone()
		proxiedTransport.Proxy = http.ProxyURL(cfg.SendProxyURLParsed)
		rt.HTTPProxied = &http.Client{Transport: proxiedTransport, Timeout: 30 * time.Second}
	}

	disallowedIPs, disallowedNets, err := cfg.ParseDisallowedNetworks()
	if err != nil {
		return nil, fmt.Errorf("error parsing disallowed networks: %w", err)
	}
	rt.HTTPAccess = httpx.NewAccessConfig(10*time.Second, disallowedIPs, disallowedNets)

	rt.Spool = dynamo.NewSpool(rt.Dynamo, rt.Config.SpoolDir+"/dynamo", 30*time.Second)
	rt.Writers = newWriters(cfg, rt.Dynamo, rt.Spool)

	return rt, nil
}

// NewTestRuntime returns a minimal Runtime wrapping the given config, suitable for tests that need a
// Runtime but don't bring up real backing services. It populates HTTP with http.DefaultClient so
// code paths that issue outbound HTTP requests work against test servers.
func NewTestRuntime(cfg *Config) *Runtime {
	return &Runtime{Config: cfg, HTTP: http.DefaultClient, HTTPProxied: http.DefaultClient}
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
