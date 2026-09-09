package firebase

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
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

func TestRegisterAtContactLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	defer testsuite.ResetDB(t, rt)

	// org 1 has one contact so record a count for it and cap the org at that
	rt.DB.MustExec(`INSERT INTO contacts_contactgroupcount(group_id, count, is_squashed) VALUES(1, 1, TRUE)`)
	rt.DB.MustExec(`UPDATE orgs_org SET limits = '{"contacts": 1}' WHERE id = 1`)
	models.FlushChannelCache()
	models.FlushContactCounts()

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, newChannel("foo-project-id", false))
	s.MountHandler(newHandler)

	req, _ := http.NewRequest(http.MethodPost, "https://localhost/c/fcm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/register", strings.NewReader("urn=12345&fcm_token=token&name=fred"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)

	// a device can't register because there's no room for its contact
	assert.Equal(t, 422, rr.Code)
	assert.JSONEq(t, `{"message":"Error","data":[{"type":"error","error":"workspace has reached its limit of 1 contacts"}]}`, rr.Body.String())
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contact WHERE org_id = 1`).Returns(1)
}
