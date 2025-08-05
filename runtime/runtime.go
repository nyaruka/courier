package runtime

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/aws/cwatch"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/s3x"
	"github.com/nyaruka/vkutil"
)

type Runtime struct {
	Config *Config
	DB     *sqlx.DB
	Dynamo *dynamodb.Client
	VK     *redis.Pool
	S3     *s3x.Service
	CW     *cwatch.Service
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

	rt.S3, err = s3x.NewService(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.S3Endpoint, cfg.S3Minio)
	if err != nil {
		return nil, fmt.Errorf("error creating S3 service: %w", err)
	}

	rt.CW, err = cwatch.NewService(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.CloudwatchNamespace, cfg.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("error creating Cloudwatch service: %w", err)
	}

	return rt, nil
}
