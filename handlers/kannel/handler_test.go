package kannel

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/vkutil/assertvk"
	"github.com/stretchr/testify/require"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}
	ignoreChs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US", []string{urns.Phone.Prefix}, map[string]any{"ignore_sent": true}),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
	RunIncomingTests(t, ignoreChs, newHandler, "testdata/incoming_ignore.json", nil)
}

func TestOutgoing(t *testing.T) {
	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			models.ConfigSendURL: "http://example.com/send",
		})
	customParamsChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			models.ConfigSendURL: "http://example.com/send?auth=foo",
		})
	nationalChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "KN", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			"use_national":       true,
			"dlr_mask":           "3",
			models.ConfigSendURL: "http://example.com/send",
		})

	opts := &OutgoingOptions{CheckRedacted: []string{"Password"}}

	RunOutgoingTests(t, defaultChannel, newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, customParamsChannel, newHandler, "testdata/outgoing_custom_params.json", opts)
	RunOutgoingTests(t, nationalChannel, newHandler, "testdata/outgoing_national.json", opts)
}

func TestUnackedLimits(t *testing.T) {
	const chUUID = "8eb23e93-5ecb-45ba-b726-3b064e0c56ab"
	const msgUUID = "0191e180-7d60-7000-aded-7d8b151cbd5b" // the UUID of every message in the outgoing test files

	var rt *runtime.Runtime
	capture := func(t *testing.T, r *runtime.Runtime) { rt = r }

	// a channel with limits tracks its successful sends, and only those, as unacknowledged
	limitedChannel := test.NewMockChannel(chUUID, "KN", "2020", "US", []string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			"max_unacked":        5000,
			"max_ack_lag":        120,
			models.ConfigSendURL: "http://example.com/send",
		})
	RunOutgoingTests(t, limitedChannel, newHandler, "testdata/outgoing.json", &OutgoingOptions{Setup: capture})

	vc := rt.VK.Get()
	assertvk.ZRange(t, vc, "{msgs}:unacked:"+chUUID, 0, -1, []string{msgUUID})
	assertvk.HGetAll(t, vc, "{msgs}:acklimits:"+chUUID, map[string]string{"max_unacked": "5000", "max_ack_lag": "120"})
	vc.Close()

	// but not if its delivery reports don't include the SMSC's acknowledgement, as nothing would clear them
	unackableChannel := test.NewMockChannel(chUUID, "KN", "2020", "US", []string{urns.Phone.Prefix},
		map[string]any{
			"password":           "Password",
			"username":           "Username",
			"use_national":       true,
			"dlr_mask":           "3",
			"max_unacked":        5000,
			models.ConfigSendURL: "http://example.com/send",
		})
	RunOutgoingTests(t, unackableChannel, newHandler, "testdata/outgoing_national.json", &OutgoingOptions{Setup: capture})

	vc = rt.VK.Get()
	assertvk.NotExists(t, vc, "{msgs}:unacked:"+chUUID)
	vc.Close()

	// the SMSC's acknowledgement clears a send even when the channel ignores sent reports, but a buffered report doesn't
	ignoreChannel := test.NewMockChannel(chUUID, "KN", "2020", "US", []string{urns.Phone.Prefix},
		map[string]any{"ignore_sent": true, "max_unacked": 5000})
	setup := func(t *testing.T, r *runtime.Runtime) {
		rt = r
		vc := r.VK.Get()
		defer vc.Close()
		require.NoError(t, queue.TrackUnacked(vc, chUUID, "019a06fa-467d-7786-b9cb-5b42177cd53f", 5000, 0))
		require.NoError(t, queue.TrackUnacked(vc, chUUID, "019a06fa-467d-7786-b9cb-5b42177cd540", 5000, 0))
	}
	RunIncomingTests(t, []*models.Channel{ignoreChannel}, newHandler, "testdata/incoming_unacked.json", &IncomingOptions{Setup: setup})

	vc = rt.VK.Get()
	assertvk.ZRange(t, vc, "{msgs}:unacked:"+chUUID, 0, -1, []string{"019a06fa-467d-7786-b9cb-5b42177cd540"})
	vc.Close()
}
