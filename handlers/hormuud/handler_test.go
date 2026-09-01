package hormuud

import (
	"fmt"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "HM", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"username": "foo@bar.com",
			"password": "sesame",
		},
	)

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"sesame"}})

	// ensure the token cached by the previous cases is cleared so these cases fetch a new one
	clearToken := func(t *testing.T, rt *runtime.Runtime) {
		rc := rt.VK.Get()
		defer rc.Close()
		redis.String(rc.Do("DEL", fmt.Sprintf("hm_token_%s", ch.UUID())))
	}

	RunOutgoingTests(t, ch, newHandler, "testdata/outgoing_token.json", &OutgoingOptions{CheckRedacted: []string{"sesame"}, Setup: clearToken})
}
