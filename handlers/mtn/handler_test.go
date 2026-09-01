package mtn

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{models.ConfigAuthToken: "customer-secret123", models.ConfigAPIKey: "customer-key"}),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func setupBackend(t *testing.T, rt *runtime.Runtime) {
	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "ACCESS_TOKEN")
}

func TestOutgoing(t *testing.T) {
	opts := &OutgoingOptions{CheckRedacted: []string{"customer-key", "customer-secret123"}, Setup: setupBackend}

	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{models.ConfigAuthToken: "customer-secret123", models.ConfigAPIKey: "customer-key"})
	RunOutgoingTests(t, defaultChannel, newHandler, "testdata/outgoing.json", opts)

	cpAddressChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MTN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{models.ConfigAuthToken: "customer-secret123", models.ConfigAPIKey: "customer-key", configCPAddress: "FOO"})
	RunOutgoingTests(t, cpAddressChannel, newHandler, "testdata/outgoing_cp_address.json", opts)
}
