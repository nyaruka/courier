package viber

import (
	"bytes"
	"crypto/hmac"
	"io"
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func addValidSignature(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	sig := calculateSignature("Token", body)
	r.Header.Set(viberSignatureHeader, string(sig))
}

func TestSignature(t *testing.T) {
	sig := calculateSignature(
		"44b935abea139fd6-53fa53b32559c4a6-12dbd3d883b06835",
		[]byte(`{"event":"unsubscribed","timestamp":1516678387902,"user_id":"KMMqtlNTDxIm/5bZhdQ5uA==","message_token":5136431130449316903}`),
	)

	if !hmac.Equal([]byte(sig), []byte("d84d8648b402a2737838fea4da41d903d1af1aed92466b1758828ad27e31a9f9")) {
		t.Errorf("hex digest not equal: %s", sig)
	}
}

func TestIncoming(t *testing.T) {
	chs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", []string{urns.Viber.Prefix}, map[string]any{
			models.ConfigAuthToken: "Token",
		}),
	}
	welcomeChs := []*models.Channel{
		test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "", []string{urns.Viber.Prefix}, map[string]any{
			models.ConfigAuthToken:    "Token",
			configViberWelcomeMessage: "Welcome to VP, Please subscribe here for more.",
		}),
	}

	opts := &IncomingOptions{Sign: addValidSignature}

	RunIncomingTests(t, chs, newHandler, "testdata/incoming.json", opts)
	RunIncomingTests(t, welcomeChs, newHandler, "testdata/incoming_welcome.json", opts)
}

func TestOutgoing(t *testing.T) {
	maxMsgLength = 160
	descriptionMaxLength = 10

	defaultChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		[]string{urns.Viber.Prefix},
		map[string]any{
			models.ConfigAuthToken: "Token",
		})
	invalidTokenChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2020", "",
		[]string{urns.Viber.Prefix},
		map[string]any{},
	)
	buttonLayoutChannel := test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "VP", "2021", "",
		[]string{urns.Viber.Prefix},
		map[string]any{
			models.ConfigAuthToken: "Token",
			"button_layout":        map[string]any{"bg_color": "#f7bb3f", "text": "<font color=\"#ffffff\">*</font><br><br>", "text_size": "large"},
		})

	opts := &OutgoingOptions{CheckRedacted: []string{"Token"}}

	RunOutgoingTests(t, defaultChannel, newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, invalidTokenChannel, newHandler, "testdata/outgoing_invalid_token.json", opts)
	RunOutgoingTests(t, buttonLayoutChannel, newHandler, "testdata/outgoing_button_layout.json", opts)
}
