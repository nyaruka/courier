package kannel

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
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
