package clogs_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/courier/utils/clogs"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogs(t *testing.T) {
	ctx := context.Background()

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"http://ivr.com/start":  {httpx.NewMockResponse(200, nil, []byte("OK"))},
		"http://ivr.com/hangup": {httpx.NewMockResponse(400, nil, []byte("Oops"))},
	}))

	clog1 := clogs.NewLog("type1", nil, []string{"sesame"})
	clog2 := clogs.NewLog("type1", nil, []string{"sesame"})

	req1, _ := httpx.NewRequest("GET", "http://ivr.com/start", nil, map[string]string{"Authorization": "Token sesame"})
	trace1, err := httpx.DoTrace(http.DefaultClient, req1, nil, nil, -1)
	require.NoError(t, err)

	clog1.HTTP(trace1)
	clog1.End()

	req2, _ := httpx.NewRequest("GET", "http://ivr.com/hangup", nil, nil)
	trace2, err := httpx.DoTrace(http.DefaultClient, req2, nil, nil, -1)
	require.NoError(t, err)

	clog2.HTTP(trace2)
	clog2.Error(clogs.NewLogError("", "", "oops"))
	clog2.End()

	assert.NotEqual(t, clog1.UUID, clog2.UUID)
	assert.NotEqual(t, time.Duration(0), clog1.Elapsed)

	ds, err := dynamo.NewService("root", "tembatemba", "us-east-1", "http://localhost:6000", "Test")
	require.NoError(t, err)

	l1 := clogs.NewLog("test_type1", nil, nil)
	l1.Error(clogs.NewLogError("code1", "ext", "message"))

	l2 := clogs.NewLog("test_type2", nil, nil)
	l2.Error(clogs.NewLogError("code2", "ext", "message"))

	// write both logs to db
	err = ds.PutItem(ctx, "ChannelLogs", l1)
	assert.NoError(t, err)
	err = ds.PutItem(ctx, "ChannelLogs", l2)
	assert.NoError(t, err)

	// read log 1 back from db
	l3 := &clogs.Log{}
	err = ds.GetItem(ctx, "ChannelLogs", map[string]types.AttributeValue{"UUID": &types.AttributeValueMemberS{Value: string(l1.UUID)}}, l3)
	assert.NoError(t, err)
	assert.Equal(t, l1.UUID, l3.UUID)
	assert.Equal(t, clogs.LogType("test_type1"), l3.Type)
	assert.Equal(t, []*clogs.LogError{clogs.NewLogError("code1", "ext", "message")}, l3.Errors)
	assert.Equal(t, l1.Elapsed, l3.Elapsed)
	assert.Equal(t, l1.CreatedOn.Truncate(time.Second), l3.CreatedOn)
}
