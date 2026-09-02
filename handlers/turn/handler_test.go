package turn

import (
	"context"
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannels = []*models.Channel{
	test.NewMockChannel(
		"8eb23e93-5ecb-45ba-b726-3b064e0c568c",
		"TRN",
		"250788383383",
		"RW",
		[]string{urns.WhatsApp.Prefix},
		map[string]any{
			"auth_token": "a123",
			"base_url":   "https://foo.bar/",
		}),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

func TestBuildAttachmentRequest(t *testing.T) {
	waHandler := newHandler(nil, channels.NewRoutes()).(*handler)
	req, _ := waHandler.BuildAttachmentRequest(context.Background(), testChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Bearer a123", req.Header.Get("Authorization"))
}

func TestOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "TRN", "12345_ID", "", []string{urns.WhatsApp.Prefix},
		map[string]any{models.ConfigAuthToken: "a123", "base_url": "https://example.org", "fb_namespace": "waba_namespace"})

	// failed media uploads are remembered in memory so each file starts with that cleared
	opts := &OutgoingOptions{CheckRedacted: []string{"a123"}, Setup: func(t *testing.T, rt *runtime.Runtime) { failedMediaCache.Clear() }}

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing_media_cache.json", opts)
	failedMediaCache.Clear()
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
