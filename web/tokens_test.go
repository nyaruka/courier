package web_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshToken(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	rt.Config.AuthToken = "sesame"
	rt.Config.InternetPort = 8180
	rt.Config.InternalPort = 8181

	server := web.NewServer(rt)
	server.Runtime().HTTP.Default.Transport = httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{
		"http://mock.com/token": {
			httpx.NewMockResponse(200, nil, []byte(`fetched-token`)),
			httpx.NewMockResponse(502, nil, []byte(`bad gateway`)),
		},
	}))
	require.NoError(t, server.Start())
	defer server.Stop()

	// a channel whose type refreshes tokens, and one whose type doesn't
	testsuite.InsertChannel(t, rt, test.NewMockChannel("f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{}))
	testsuite.InsertChannel(t, rt, test.NewMockChannel("53e5aafa-8155-449d-9009-fcb30d54bd26", "XX", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{}))

	submit := func(body, authToken string) (int, string) {
		req, _ := http.NewRequest("POST", "http://localhost:8181/ci/token/refresh", strings.NewReader(body))
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(respBody)
	}

	countLogs := func() int {
		count := 0
		for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
			if strings.HasPrefix(item.Key.SK, "log#") && item.Data["type"] == "token_refresh" {
				count++
			}
		}
		return count
	}

	statusCode, _ := submit(`{}`, "")
	assert.Equal(t, 401, statusCode)

	statusCode, respBody := submit(`{}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, respBody, `Field validation for 'ChannelType' failed on the 'required' tag`)
	assert.Contains(t, respBody, `Field validation for 'ChannelUUID' failed on the 'required' tag`)

	statusCode, respBody = submit(`{"channel_type": "MCK", "channel_uuid": "9aa5ab57-1e22-4e8f-ae4d-de8a8b5d3e45"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, respBody, `error getting channel`)

	statusCode, respBody = submit(`{"channel_type": "XX", "channel_uuid": "53e5aafa-8155-449d-9009-fcb30d54bd26"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, respBody, `channel type XX doesn't support access tokens`)

	// a refresh is logged, with the credentials used for it redacted
	statusCode, respBody = submit(`{"channel_type": "MCK", "channel_uuid": "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b"}`, "sesame")
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"token": "fetched-token"}`, respBody)

	// a failed refresh is an error, and is also logged
	statusCode, respBody = submit(`{"channel_type": "MCK", "channel_uuid": "f8be89c7-58b5-4d3c-8e5c-c0d049f4d43b"}`, "sesame")
	assert.Equal(t, 400, statusCode)
	assert.Contains(t, respBody, `unable to fetch token`)

	require.Eventually(t, func() bool { return countLogs() == 2 }, 5*time.Second, 100*time.Millisecond)

	for _, item := range dyntest.ScanAll(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table()) {
		if item.Data["type"] == "token_refresh" {
			data, err := item.GetData()
			require.NoError(t, err)
			assert.NotContains(t, string(jsonx.MustMarshal(data)), "sesame")
			assert.Contains(t, string(jsonx.MustMarshal(data)), "mock.com/token")
		}
	}
}
