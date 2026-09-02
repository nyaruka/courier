package jiochat

import (
	"context"
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "JC", "2020", "US", []string{urns.JioChat.Prefix}, map[string]any{configAppSecret: "secret123", configAppID: "app-id"}),
}

func setupBackend(t *testing.T, rt *runtime.Runtime) {
	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestDescribeURN(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)
	setupBackend(t, rt)

	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://channels.jiochat.com/user/info.action?openid=1337": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"nickname": "John Doe"}`)),
		},
		"https://channels.jiochat.com/user/info.action?openid=4567": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"nickname": ""}`)),
		},
	})

	s := web.NewServer(rt)
	handler := s.MountHandler(newHandler).(*handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"jiochat:1337", map[string]string{"name": "John Doe"}},
		{"jiochat:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.DescribeURN(context.Background(), testChannels[0], tc.urn, clog)
		assert.Equal(t, tc.expectedMetadata, metadata)
	}

	// lookups are made with the cached access token
	assert.Len(t, clog.HttpLogs, 2)
	assert.Contains(t, clog.HttpLogs[0].Request, "Authorization: Bearer ACCESS_TOKEN")

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

func TestBuildAttachmentRequest(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	// ensure that we start with no cached token
	testsuite.ResetValkey(t, rt)

	s := web.NewServer(rt)
	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://channels.jiochat.com/auth/token.action": {
			httpx.NewMockResponse(http.StatusOK, nil, []byte(`{"access_token": "SESAME"}`)),
		},
	})
	handler := s.MountHandler(newHandler).(*handler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))

	// check that request has the fetched access token
	req, err := handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=12", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=12", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))

	// and that we have a log for that request
	assert.Len(t, clog.HttpLogs, 1)
	assert.Equal(t, "https://channels.jiochat.com/auth/token.action", clog.HttpLogs[0].URL)

	// check that another request reads token from cache
	req, err = handler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://channels.jiochat.com/media/download.action?media_id=13", clog)
	assert.NoError(t, err)
	assert.Equal(t, "https://channels.jiochat.com/media/download.action?media_id=13", req.URL.String())
	assert.Equal(t, "Bearer SESAME", req.Header.Get("Authorization"))
	assert.Len(t, clog.HttpLogs, 1)

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", &IncomingOptions{Setup: setupBackend})
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160

	RunOutgoingTests(t, testChannels[0], newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"secret123"}, Setup: setupBackend})
}
