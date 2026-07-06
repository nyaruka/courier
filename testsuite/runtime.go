package testsuite

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path"
	"testing"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/centrifugo"
	"github.com/stretchr/testify/require"
)

const (
	postgresSchemaPath = "./testsuite/testdata/schema.sql"
	postgresDataPath   = "./testsuite/testdata/data.sql"
	dynamoTablesPath   = "./testsuite/testdata/dynamo.json"
)

func Runtime(t *testing.T) (context.Context, *runtime.Runtime) {
	cfg := runtime.NewDefaultConfig()
	cfg.DB = "postgres://courier_test:temba@postgres:5432/courier_test?sslmode=disable"
	cfg.Valkey = "valkey://valkey:6379/0"
	cfg.MediaDomain = "nyaruka.s3.com"

	// AWS credentials and region are resolved from the standard SDK default chain, so export them as
	// the standard env vars (localstack values) rather than via courier config
	t.Setenv("AWS_ACCESS_KEY_ID", "root")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tembatemba")
	t.Setenv("AWS_REGION", "us-east-1")

	// configure S3 to use a localstack instance
	cfg.S3Endpoint = "http://localstack:4566"
	cfg.S3AttachmentsBucket = "test-attachments"
	cfg.S3PathStyle = true
	cfg.DynamoEndpoint = "http://localstack:4566"
	cfg.DynamoTablePrefix = "Test"
	cfg.SpoolDir = absPath("./_test_spool")

	rt, err := runtime.NewRuntime(cfg)
	require.NoError(t, err)

	// create Postgres tables if necessary
	_, err = rt.DB.Exec("SELECT * from orgs_org")
	if err != nil {
		ResetDB(t, rt)
	}

	// create Dynamo tables if necessary
	dyntest.CreateTables(t, rt.Dynamo, absPath(dynamoTablesPath), false)

	rt.Centrifugo = centrifugo.NewService(centrifugo.NewMockClient(), rt.VK)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	t.Cleanup(func() {
		rt.DB.Close()
		rt.VK.Close()
	})

	return t.Context(), rt
}

func ResetDB(t *testing.T, rt *runtime.Runtime) {
	rt.DB.MustExec(string(ReadFile(t, absPath(postgresSchemaPath))))
	rt.DB.MustExec(string(ReadFile(t, absPath(postgresDataPath))))
}

func ResetValkey(t *testing.T, rt *runtime.Runtime) {
	r := rt.VK.Get()
	defer r.Close()

	_, err := r.Do("FLUSHDB")
	require.NoError(t, err)
}

// CentrifugoHistory returns the JSON payloads published to the given Centrifugo channel, oldest first. The runtime's
// Centrifugo client is a mock so this reads back what the test published rather than hitting a real server.
func CentrifugoHistory(t *testing.T, rt *runtime.Runtime, channel string) []json.RawMessage {
	t.Helper()

	var history []json.RawMessage
	for _, p := range rt.Centrifugo.Client.(*centrifugo.MockClient).Publications() {
		if p.Channel == channel {
			// Publication.Data is any but the mock records publications with their data re-marshaled
			history = append(history, p.Data.(json.RawMessage))
		}
	}
	return history
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

func ReadFile(t *testing.T, path string) []byte {
	t.Helper()

	d, err := os.ReadFile(path)
	require.NoError(t, err)
	return d
}
