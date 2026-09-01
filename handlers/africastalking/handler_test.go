package africastalking

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigUsername: "Username",
			models.ConfigAPIKey:   "KEY",
		})
	sharedChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigUsername: "Username",
			models.ConfigAPIKey:   "KEY",
			configIsShared:        true,
		})
	customSendURLChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AT", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigUsername: "Username",
			models.ConfigAPIKey:   "KEY",
			models.ConfigSendURL:  "https://other.example.com/send",
		})

	opts := &OutgoingOptions{CheckRedacted: []string{"KEY"}}

	RunOutgoingTests(t, defaultChannel, newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, sharedChannel, newHandler, "testdata/outgoing_shared.json", opts)
	RunOutgoingTests(t, customSendURLChannel, newHandler, "testdata/outgoing_custom_send_url.json", opts)
}
