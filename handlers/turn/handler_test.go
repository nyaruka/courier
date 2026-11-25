package turn

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testChannels = []*models.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"TRN",
		"250788383383",
		"RW",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			"base_url": "https://foo.bar/",
			"username": "bob",
			"password": "sesame",
		}),
}

// caches an access token for the given channel as though we'd already logged in
func setCachedToken(t *testing.T, rt *runtime.Runtime, ch *models.Channel, token string) {
	rc := rt.VK.Get()
	defer rc.Close()

	_, err := rc.Do("SET", fmt.Sprintf("channel-token:%s", ch.UUID()), token, "EX", 3600)
	require.NoError(t, err)
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

func TestBuildAttachmentRequest(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)
	setCachedToken(t, rt, testChannels[0], "a123")

	h := web.NewServer(rt).MountHandler(newHandler).(*handler)
	clog := models.NewChannelLog(models.ChannelLogTypeAttachmentFetch, testChannels[0], nil, h.RedactValues(testChannels[0]))

	req, err := h.BuildAttachmentRequest(context.Background(), testChannels[0], "https://example.org/v1/media/41", clog)
	require.NoError(t, err)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer a123", req.Header.Get("Authorization"))
}

func TestAccessTokens(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetValkey(t, rt)

	ch := testChannels[0]
	h := web.NewServer(rt).MountHandler(newHandler).(*handler)

	tokenTTL := func() time.Duration {
		rc := rt.VK.Get()
		defer rc.Close()
		ttl, err := redis.Int(rc.Do("TTL", fmt.Sprintf("channel-token:%s", ch.UUID())))
		require.NoError(t, err)
		return time.Duration(ttl) * time.Second
	}
	expiresAfter := func(d time.Duration) string {
		return time.Now().Add(d).UTC().Format("2006-01-02 15:04:05+00:00")
	}

	rt.HTTP.Default.Transport = test.MockTransport(map[string][]*httpx.MockResponse{
		"https://foo.bar/v1/users/login": {
			httpx.NewMockResponse(200, nil, []byte(`{"users": [{"token": "token1", "expires_after": "`+expiresAfter(7*24*time.Hour)+`"}]}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"users": [{"token": "token2", "expires_after": "`+expiresAfter(3*time.Hour)+`"}]}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"users": [{"token": "token3"}]}`)),
			httpx.NewMockResponse(401, nil, []byte(`{"errors": [{"code": 1005, "title": "Access denied"}]}`)),
			httpx.NewMockResponse(200, nil, []byte(`{"users": []}`)),
			httpx.NewMockResponse(503, nil, []byte(`unavailable`)),
		},
	})

	newLog := func() *models.ChannelLog {
		return models.NewChannelLog(models.ChannelLogTypeTokenRefresh, ch, nil, h.RedactValues(ch))
	}

	// no cached token so we log in, and cache the token for well under its 7 day lifetime
	clog := newLog()
	token, err := h.getAccessToken(ctx, ch, clog)
	assert.NoError(t, err)
	assert.Equal(t, "token1", token)
	assert.Len(t, clog.HttpLogs, 1)
	assert.InDelta(t, 24*time.Hour, tokenTTL(), float64(5*time.Second))
	AssertChannelLogRedaction(t, clog, []string{"sesame", "Ym9iOnNlc2FtZQ=="})

	// next call uses the cached token
	clog = newLog()
	token, err = h.getAccessToken(ctx, ch, clog)
	assert.NoError(t, err)
	assert.Equal(t, "token1", token)
	assert.Len(t, clog.HttpLogs, 0)

	// refreshing logs in regardless of the cached token and replaces it - this time with a token which expires sooner
	// so is cached for less time
	clog = newLog()
	token, err = h.RefreshAccessToken(ctx, ch, clog)
	assert.NoError(t, err)
	assert.Equal(t, "token2", token)
	assert.Len(t, clog.HttpLogs, 1)
	assert.InDelta(t, 2*time.Hour, tokenTTL(), float64(5*time.Second))

	clog = newLog()
	token, err = h.getAccessToken(ctx, ch, clog)
	assert.NoError(t, err)
	assert.Equal(t, "token2", token)
	assert.Len(t, clog.HttpLogs, 0)

	// a token without an expiry is cached for our max
	h.clearAccessToken(ch)
	token, err = h.getAccessToken(ctx, ch, newLog())
	assert.NoError(t, err)
	assert.Equal(t, "token3", token)
	assert.InDelta(t, 24*time.Hour, tokenTTL(), float64(5*time.Second))

	// rejected credentials
	h.clearAccessToken(ch)
	_, err = h.getAccessToken(ctx, ch, newLog())
	assert.EqualError(t, err, "login rejected with status 401")

	// a response without a token
	clog = newLog()
	_, err = h.getAccessToken(ctx, ch, clog)
	assert.EqualError(t, err, "login response missing token")
	assert.Equal(t, "response_value_missing", clog.Errors[0].Code)

	// login endpoint unavailable
	_, err = h.getAccessToken(ctx, ch, newLog())
	assert.Equal(t, errLoginUnavailable, err)
}

func TestOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TRN", "12345_ID", "", []string{urns.WhatsApp.Prefix},
		map[string]any{"base_url": "https://example.org", "username": "bob", "password": "sesame", "fb_namespace": "waba_namespace"})

	// failed media uploads are remembered in memory so each file starts with that cleared
	opts := &OutgoingOptions{
		CheckRedacted: []string{"a123", "sesame"},
		Setup: func(t *testing.T, rt *runtime.Runtime) {
			failedMediaCache.Clear()
			setCachedToken(t, rt, ch, "a123")
		},
		FreeURLs: []string{"https://foo.bar/", "https://example.org/v1/media"},
	}

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing_media_cache.json", opts)
	failedMediaCache.Clear()

	// a send without a cached token logs in first, and a send whose token is rejected drops it
	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing_login.json", &OutgoingOptions{
		CheckRedacted: []string{"sesame", "Ym9iOnNlc2FtZQ=="},
		FreeURLs:      []string{"https://example.org/v1/users/login"},
	})
}

func TestGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, "en", getSupportedLanguage(i18n.NilLocale))
	assert.Equal(t, "en", getSupportedLanguage(i18n.Locale("eng")))
	assert.Equal(t, "en_US", getSupportedLanguage(i18n.Locale("eng-US")))
	assert.Equal(t, "pt_PT", getSupportedLanguage(i18n.Locale("por")))
	assert.Equal(t, "pt_PT", getSupportedLanguage(i18n.Locale("por-PT")))
	assert.Equal(t, "pt_BR", getSupportedLanguage(i18n.Locale("por-BR")))
	assert.Equal(t, "fil", getSupportedLanguage(i18n.Locale("fil")))
	assert.Equal(t, "fr", getSupportedLanguage(i18n.Locale("fra-CA")))
	assert.Equal(t, "en", getSupportedLanguage(i18n.Locale("run")))
}
