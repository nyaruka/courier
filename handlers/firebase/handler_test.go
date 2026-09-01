package firebase

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
)

func newChannel(projectID string, notification bool) *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCM", "1234", "",
		[]string{urns.Firebase.Prefix},
		map[string]any{
			configNotification: notification,
			configTitle:        "FCMTitle",
			configCredentialsFile: map[string]any{
				"type":                        "service_account",
				"project_id":                  projectID,
				"private_key_id":              "123",
				"private_key":                 "BLAH",
				"client_email":                "foo@example.com",
				"client_id":                   "123123",
				"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
				"token_uri":                   "https://oauth2.googleapis.com/token",
				"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
				"client_x509_cert_url":        "",
				"universe_domain":             "googleapis.com",
			},
		})
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{newChannel("foo-project-id", false)}, newHandler, "testdata/incoming.json", nil)
}

func setupBackend(t *testing.T, rt *runtime.Runtime) {
	// ensure there's a cached access token
	rc := rt.VK.Get()
	defer rc.Close()
	rc.Do("SET", "channel-token:8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCMToken")
}

func TestOutgoing(t *testing.T) {
	opts := &OutgoingOptions{Setup: setupBackend}

	RunOutgoingTests(t, newChannel("foo-project-id", false), newHandler, "testdata/outgoing.json", opts)
	RunOutgoingTests(t, newChannel("bar-project-id", true), newHandler, "testdata/outgoing_notification.json", opts)
}
