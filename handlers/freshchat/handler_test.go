package freshchat

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

const cert = "-----BEGIN RSA PUBLIC KEY----- MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAuGJLF4hTTtxWogT6dNkGf3CEgLAR2mGJzlds5cNbrHFoJNFnmVhkRYGzLYxx4EtDiezNCZVHfyMI2AKuNSQW2fEdDatVIG+q3Zr/X9eeDl8kQOGy804J/fgCYDrN8RQu0n5Dh1inv4puca0wb29SCvoAwrWb33ehDBIvv6+rUKBdjtv2xTV65kNiVDo5VRCaYRVeE10osxeONgw55HVY4nczuxnR+dmc2282de6WHe5LXtr0ZBdJ8yttFOLIluZ/sNM5DIWZBkIWQhyT581tbA7bTpsIbrT/IMBlmioIILw8WGtI7zcmNkjU5dnq5HnlVKEDhj/Ug/dLiyno8+Vp7QIDAQAB -----END RSA PUBLIC KEY-----"

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FC", "2020", "US", []string{urns.FreshChat.Prefix}, map[string]any{
			"username":   "c8fddfaf-622a-4a0e-b060-4f3ccbeab606", // agent_id
			"secret":     cert,                                   // public_key for sig
			"auth_token": "authtoken",                            // API bearer token
		}),
	}

	RunIncomingTests(t, chs, newHandler("FC", "FreshChat", true), "testdata/incoming.json", nil)
	RunIncomingTests(t, chs, newHandler("FC", "FreshChat", false), "testdata/incoming_no_validation.json", nil)
}

func TestOutgoing(t *testing.T) {
	ch := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "FC", "2020", "US", []string{urns.FreshChat.Prefix}, map[string]any{
		"username":   "c8fddfaf-622a-4a0e-b060-4f3ccbeab606",
		"secret":     cert,
		"auth_token": "enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ=",
	})

	RunOutgoingTests(t, ch, newHandler("FC", "FreshChat", false), "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{cert, "enYtdXNlcm5hbWU6enYtcGFzc3dvcmQ="}})
}
