package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var instgramTestChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "IG", "1234567890", "", []string{urns.Instagram.Prefix}, map[string]any{models.ConfigAuthToken: "a123"}),
}

func TestInstagramIncoming(t *testing.T) {
	RunIncomingTests(t, instgramTestChannels, newHandler("IG", "Instagram"), "testdata/instagram_incoming.json", &IncomingOptions{Sign: addValidSignature, NoInvalidChannelCheck: true})
	RunIncomingTests(t, instgramTestChannels, newHandler("IG", "Instagram"), "testdata/instagram_verify.json", &IncomingOptions{NoInvalidChannelCheck: true})
}

func TestInstagramOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IG", "12345", "", []string{urns.Instagram.Prefix}, map[string]any{models.ConfigAuthToken: "a123"})

	checkRedacted := []string{"wac_admin_system_user_token", "missing_facebook_app_secret", "missing_facebook_webhook_secret", "a123"}

	RunOutgoingTests(t, ch, newHandler("IG", "Instagram"), "testdata/instagram_outgoing.json", &OutgoingOptions{CheckRedacted: checkRedacted})
}

func TestInstagramDescribeURN(t *testing.T) {
	defer func(u string) { graphURL = u }(graphURL)
	fbGraph := buildMockFBGraphIG()
	defer fbGraph.Close()

	channel := instgramTestChannels[0]
	handler := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig())).MountHandler(newHandler("IG", "Instagram"))
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, channel, nil, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"instagram:1337", map[string]string{"name": "John Doe"}},
		{"instagram:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(models.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "wac_admin_system_user_token"})
}

func TestInstagramBuildAttachmentRequest(t *testing.T) {
	s := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig()))

	handler := s.MountHandler(newHandler("IG", "Instagram")).(*handler)
	req, _ := handler.BuildAttachmentRequest(context.Background(), facebookTestChannels[0], "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, http.Header{}, req.Header)
}

// mocks the call to the Facebook graph API
func buildMockFBGraphIG() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", http.StatusForbidden)
		}

		// user has a name
		if strings.HasSuffix(r.URL.Path, "1337") {
			w.Write([]byte(`{ "name": "John Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "name": ""}`))
	}))
	graphURL = server.URL

	return server
}
