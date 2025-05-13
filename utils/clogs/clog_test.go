package clogs_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/courier/utils/clogs"
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

	clog1 := clogs.New("type1", nil, []string{"sesame"})
	clog2 := clogs.New("type1", nil, []string{"sesame"})

	req1, _ := httpx.NewRequest(ctx, "GET", "http://ivr.com/start", nil, map[string]string{"Authorization": "Token sesame"})
	trace1, err := httpx.DoTrace(http.DefaultClient, req1, nil, nil, -1)
	require.NoError(t, err)

	clog1.HTTP(trace1)
	clog1.End()

	req2, _ := httpx.NewRequest(ctx, "GET", "http://ivr.com/hangup", nil, nil)
	trace2, err := httpx.DoTrace(http.DefaultClient, req2, nil, nil, -1)
	require.NoError(t, err)

	clog2.HTTP(trace2)
	clog2.Error(&clogs.Error{Message: "oops"})
	clog2.End()

	assert.NotEqual(t, clog1.UUID, clog2.UUID)
	assert.NotEqual(t, time.Duration(0), clog1.Elapsed)

	l1 := clogs.New("test_type1", nil, nil)
	l1.Error(&clogs.Error{Code: "code1", ExtCode: "ext", Message: "message"})

	l2 := clogs.New("test_type2", nil, nil)
	l2.Error(&clogs.Error{Code: "code2", ExtCode: "ext", Message: "message"})
}
