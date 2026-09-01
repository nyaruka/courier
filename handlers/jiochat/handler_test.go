package jiochat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func buildMockJCAPI() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")
		defer r.Body.Close()

		// a request for an access token is the one request that doesn't carry one
		if strings.HasSuffix(r.URL.Path, "auth/token.action") {
			w.Write([]byte(`{"access_token": "ACCESS_TOKEN"}`))
			return
		}

		if authorizationHeader != "Bearer ACCESS_TOKEN" {
			http.Error(w, "invalid file", http.StatusForbidden)
			return
		}

		if strings.HasSuffix(r.URL.Path, "user/info.action") {
			openID := r.URL.Query().Get("openid")

			// user has a name
			if strings.HasSuffix(openID, "1337") {
				w.Write([]byte(`{ "nickname": "John Doe"}`))
				return
			}

			// no name
			w.Write([]byte(`{ "nickname": ""}`))

		}

	}))
	sendURL = server.URL

	return server
}

func TestDescribeURN(t *testing.T) {
	defer func(u string) { sendURL = u }(sendURL)
	JCAPI := buildMockJCAPI()
	defer JCAPI.Close()

	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)

	// use a plain client so the handler can reach the mock API on localhost
	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")

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
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"secret123"})
}

func TestBuildAttachmentRequest(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	// reset send URL
	sendURL = "https://channels.jiochat.com"

	// ensure that we start with no cached token
	testsuite.ResetValkey(t, rt)

	rt.HTTP.Default = &http.Client{Transport: httpx.WithTraces(nil), Timeout: 30 * time.Second}
	rt.HTTP.Proxied = rt.HTTP.Default

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
