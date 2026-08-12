package runtime

import (
	"context"
	"fmt"

	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/nyaruka/vkutil"
	"github.com/vinovest/sqlx"
)

type Runtime struct {
	Config     *Config
	DB         *sqlx.DB
	Dynamo     *Dynamo
	VK         *redis.Pool
	S3         *s3x.Service
	CW         *cwatch.Service
	Centrifugo *centrifugo.Service

	HTTP  *HTTP
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

	rt.HTTP, err = newHTTP(cfg)
	if err != nil {
		return nil, err
	}

	rt.Stats = NewStatsCollector()

	return rt, nil
}

// NewTestRuntime returns a minimal Runtime wrapping the given config, suitable for tests that need a
// Runtime but don't bring up real backing services. It populates HTTP with dedicated clients so code paths
// that issue outbound HTTP requests work against test servers, and so tests can install a mocking transport
// via httpx.WithMocks without mutating http.DefaultClient.
func NewTestRuntime(cfg *Config) *Runtime {
	return &Runtime{
		Config: cfg,
		HTTP:   newTestHTTP(),
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
