package bandwidth

import (
	"context"
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func newChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "BW", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{models.ConfigUsername: "user1", models.ConfigPassword: "pass1", configAccountID: "accound-id", configMsgApplicationID: "application-id"},
	)
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newChannel()}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, newChannel(), newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{httpx.BasicAuth("user1", "pass1")}})
}

func TestBuildAttachmentRequest(t *testing.T) {
	bwHandler := newHandler(nil, channels.NewRoutes()).(*handler)
	req, _ := bwHandler.BuildAttachmentRequest(context.Background(), newChannel(), "https://example.org/v1/media/41", nil)
	assert.Equal(t, "https://example.org/v1/media/41", req.URL.String())
	assert.Equal(t, "Basic dXNlcjE6cGFzczE=", req.Header.Get("Authorization"))
}
