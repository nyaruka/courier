package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

const channelUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c568c"

var testChannels = []*models.Channel{
	test.NewMockChannel(channelUUID, "SL", "2022", "US", []string{urns.Slack.Prefix}, map[string]any{"bot_token": "xoxb-abc123", "verification_token": "one-long-verification-token"}),
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	opts := &OutgoingOptions{CheckRedacted: []string{"xoxb-abc123", "one-long-verification-token"}}

	RunOutgoingTests(t, testChannels[0], newHandler, "testdata/outgoing.json", opts)
}

func buildMockSlackService() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users.info" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"user":{"real_name":"dummy user"}}`))
		}
	}))

	apiURL = server.URL

	return server
}

func TestDescribeURN(t *testing.T) {
	defer func(u string) { apiURL = u }(apiURL)
	server := buildMockSlackService()
	defer server.Close()

	handler := web.NewServer(runtime.NewTestRuntime(runtime.NewDefaultConfig())).MountHandler(newHandler)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, testChannels[0], nil, handler.RedactValues(testChannels[0]))
	urn, _ := urns.New(urns.Slack, "U012345")

	data := map[string]string{"name": "dummy user"}

	describe, err := handler.(models.URNDescriber).DescribeURN(context.Background(), testChannels[0], urn, clog)
	assert.Nil(t, err)
	assert.Equal(t, data, describe)

	AssertChannelLogRedaction(t, clog, []string{"xoxb-abc123", "one-long-verification-token"})
}
