package infobip

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
)

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US", []string{urns.Phone.Prefix}, nil),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", nil)
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, maxWebhookBodyLen)
	mac := hmac.New(sha256.New, []byte("sesame"))
	mac.Write(body)
	r.Header.Set(signatureHeader, signaturePrefix+hex.EncodeToString(mac.Sum(nil)))
}

func TestIncomingSigned(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US", []string{urns.Phone.Prefix},
			map[string]any{models.ConfigSecret: "sesame"}),
	}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming_signed.json", &IncomingOptions{Sign: addValidSignature})
}

func TestOutgoing(t *testing.T) {
	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
		})
	transChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigPassword: "Password",
			models.ConfigUsername: "Username",
			configTransliteration: "COLOMBIAN",
		})
	apiKeyChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigAPIKey: "test-api-key",
		})
	platformChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "IB", "2020", "US",
		[]string{urns.Phone.Prefix},
		map[string]any{
			models.ConfigAPIKey: "test-api-key",
			configApplicationID: "my-app",
			configEntityID:      "my-entity",
		})

	basicOpts := &OutgoingOptions{CheckRedacted: []string{httpx.BasicAuth("Username", "Password")}}
	apiKeyOpts := &OutgoingOptions{CheckRedacted: []string{"App test-api-key"}}

	RunOutgoingTests(t, defaultChannel, newHandler, "testdata/outgoing.json", basicOpts)
	RunOutgoingTests(t, transChannel, newHandler, "testdata/outgoing_transliteration.json", basicOpts)
	RunOutgoingTests(t, apiKeyChannel, newHandler, "testdata/outgoing_api_key.json", apiKeyOpts)
	RunOutgoingTests(t, platformChannel, newHandler, "testdata/outgoing_platform.json", apiKeyOpts)
}
