package dart

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, NewHandler("DA", "DartMedia", sendURL, maxMsgLength), "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160

	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "DA", "2020", "ID",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigUsername: "Username",
			models.ConfigPassword: "Password",
		})

	RunOutgoingTests(t, ch, NewHandler("DA", "Dartmedia", sendURL, maxMsgLength), "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"Password"}})
}
