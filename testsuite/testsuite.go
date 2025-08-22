package testsuite

import (
	"context"
	"log/slog"
	"os"
	"path"
	"testing"

	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/stretchr/testify/require"
)

const (
	dynamoTablesPath = "./testsuite/testdata/dynamo.json"
)

func Runtime(t *testing.T) (context.Context, *runtime.Runtime) {
	cfg := runtime.NewDefaultConfig()
	cfg.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	cfg.Valkey = "valkey://localhost:6379/0"
	cfg.MediaDomain = "nyaruka.s3.com"

	// configure S3 to use a local minio instance
	cfg.AWSAccessKeyID = "root"
	cfg.AWSSecretAccessKey = "tembatemba"
	cfg.S3Endpoint = "http://localhost:9000"
	cfg.S3AttachmentsBucket = "test-attachments"
	cfg.S3Minio = true
	cfg.DynamoEndpoint = "http://localhost:6000"
	cfg.DynamoTablePrefix = "Test"

	rt, err := runtime.NewRuntime(cfg)
	require.NoError(t, err)

	// create Dynamo tables if necessary
	dyntest.CreateTables(t, rt.Dynamo, absPath(dynamoTablesPath), false)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	return t.Context(), rt
}

func ResetValkey(t *testing.T, rt *runtime.Runtime) {
	r := rt.VK.Get()
	defer r.Close()

	_, err := r.Do("FLUSHDB")
	require.NoError(t, err)
}

// Converts a project root relative path to an absolute path usable in any test. This is needed because go tests
// are run with a working directory set to the current module being tested.
func absPath(p string) string {
	// start in working directory and go up until we are in a directory containing go.mod
	dir, _ := os.Getwd()
	for dir != "/" {
		if _, err := os.Stat(path.Join(dir, "go.mod")); err == nil {
			break
		}
		dir = path.Dir(dir)
	}
	return path.Join(dir, p)
}
