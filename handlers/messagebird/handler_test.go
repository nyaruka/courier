package messagebird

import (
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

// signs a request the way MessageBird does, with a JWT carrying hashes of the URL and body
func addValidSignature(r *http.Request) {
	body, _ := ReadBody(r, maxRequestBodyBytes)
	bodysig := calculateSignature(body)
	urlsig := calculateSignature([]byte("https://localhost" + r.URL.Path))
	t := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"iss":          "MessageBird",
			"nbf":          1690306305,
			"jti":          "e92cf079-362d-4813-ab40-bbdd938bdc6d",
			"payload_hash": bodysig,
			"url_hash":     urlsig,
		})

	signedJWT, _ := t.SignedString([]byte("my_super_secret"))
	r.Header.Set(signatureHeader, signedJWT)
}

func newChannel() *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "MBD", "18005551212", "US", []string{urns.Phone.Prefix}, map[string]any{
		"secret":     "my_super_secret", // secret key to sign for sig
		"auth_token": "authtoken",       // API bearer token
	})
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newChannel()}, newHandler("MBD", "Messagebird", true), "testdata/incoming.json", &IncomingOptions{Sign: addValidSignature})
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, newChannel(), newHandler("MBD", "Messagebird", false), "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"my_super_secret", "authtoken"}})
}
