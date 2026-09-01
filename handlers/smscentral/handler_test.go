package smscentral

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
			[]string{urns.Phone.Prefix},
			map[string]any{"username": "Username", "password": "Password"}),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "SC", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
		})

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"Password"}})
}
