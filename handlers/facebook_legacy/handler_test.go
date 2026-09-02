package facebook_legacy

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

var testChannels = []*models.Channel{
	test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FB", "1234", "",
		[]string{urns.Facebook.Prefix},
		map[string]any{models.ConfigAuthToken: "a123", models.ConfigSecret: "mysecret"}),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	// shorter max msg length for testing
	maxMsgLength = 100

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FB", "2020", "US", []string{urns.Facebook.Prefix}, map[string]any{models.ConfigAuthToken: "access_token"})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"access_token"}})
}

// mocks the call to the Facebook graph API
func buildMockFBGraph() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.URL.Query().Get("access_token")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "a123" {
			http.Error(w, "invalid auth token", http.StatusForbidden)
		}

		// user has a name
		if strings.HasSuffix(r.URL.Path, "1337") {
			w.Write([]byte(`{ "first_name": "John", "last_name": "Doe"}`))
			return
		}

		// no name
		w.Write([]byte(`{ "first_name": "", "last_name": ""}`))
	}))
	graphURL = server.URL

	return server
}

func TestDescribeURN(t *testing.T) {
	defer func(u string) { graphURL = u }(graphURL)
	fbGraph := buildMockFBGraph()
	defer fbGraph.Close()

	channel := testChannels[0]
	handler := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig())).MountHandler(newHandler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, channel, nil, handler.RedactValues(channel))

	tcs := []struct {
		urn              urns.URN
		expectedMetadata map[string]string
	}{
		{"facebook:1337", map[string]string{"name": "John Doe"}},
		{"facebook:4567", map[string]string{"name": ""}},
	}

	for _, tc := range tcs {
		metadata, _ := handler.(models.URNDescriber).DescribeURN(context.Background(), channel, tc.urn, clog)
		assert.Equal(t, metadata, tc.expectedMetadata)
	}

	AssertChannelLogRedaction(t, clog, []string{"a123", "mysecret"})
}
