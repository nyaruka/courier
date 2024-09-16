package clogs_test

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamo(t *testing.T) {
	ctx := context.Background()
	ds, err := dynamo.NewService("root", "tembatemba", "us-east-1", "http://localhost:6000", "Test")
	require.NoError(t, err)

	l1 := clogs.NewLog("test_type1", nil, nil)
	l1.Error(clogs.NewLogError("code1", "ext", "message"))

	l2 := clogs.NewLog("test_type2", nil, nil)
	l2.Error(clogs.NewLogError("code2", "ext", "message"))

	// write both logs to db
	err = clogs.BulkPut(ctx, ds, []*clogs.Log{l1, l2})
	assert.NoError(t, err)

	// read log 1 back from db
	l3, err := clogs.Get(ctx, ds, l1.UUID)
	assert.NoError(t, err)
	assert.Equal(t, l1.UUID, l3.UUID)
	assert.Equal(t, clogs.LogType("test_type1"), l3.Type)
	assert.Equal(t, []*clogs.LogError{clogs.NewLogError("code1", "ext", "message")}, l3.Errors)
	assert.Equal(t, l1.Elapsed, l3.Elapsed)
	assert.Equal(t, l1.CreatedOn.Truncate(time.Second), l3.CreatedOn)
}
