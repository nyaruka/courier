package chip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/courier/v26/web"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/core/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	channelUUID = "0665bf36-4d2e-4c3f-b8a1-9f8e6a5c2d71"
	startURL    = "/c/chp/" + channelUUID + "/start"

	testChatID = "vM0GGhDrqpTQefIEinK0up3C"
)

var testChannels = []*models.Channel{
	test.NewMockChannel(channelUUID, "CHP", "", "", []string{urns.WebChat.Prefix}, nil),
}

// the webchat service is configured with the token its requests have to carry and the address we post to
func setup(t *testing.T, rt *runtime.Runtime) {
	rt.Config.ChipURL = "http://chip"
	rt.Config.ChipAuthToken = "sesame"
}

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, testChannels, newHandler, "testdata/incoming.json", &IncomingOptions{Setup: setup})
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, testChannels[0], newHandler, "testdata/outgoing.json", &OutgoingOptions{Setup: setup})
}

// posts to the handler on a server with the channel mounted
func post(t *testing.T, rt *runtime.Runtime, path, body string) (int, string) {
	t.Helper()

	s := web.NewServer(rt)
	testsuite.InsertChannel(t, rt, testChannels[0])
	s.MountHandler(newHandler)

	req, _ := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(body))
	if token := rt.Config.ChipAuthToken; token != "" {
		req.Header.Set("Authorization", "Token "+token)
	}
	rr := httptest.NewRecorder()
	s.Router().ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

// with no token configured nothing can authenticate, rather than everything: the widget's requests must come through
// the service, where they're throttled and origin-checked
func TestAuthRequired(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	code, body := post(t, rt, startURL, `{"chat_id": "`+testChatID+`"}`)
	assert.Equal(t, 401, code, body)
	assert.JSONEq(t, `{"message":"Error","data":[{"type":"error","error":"invalid or missing authorization header"}]}`, body)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contacturn WHERE identity = $1`, "webchat:"+testChatID).Returns(0)
}

func TestStartAtContactLimit(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	defer testsuite.ResetDB(t, rt)

	// org 1 has one contact so record a count for it and cap the org at that
	rt.DB.MustExec(`INSERT INTO contacts_contactgroupcount(group_id, count, is_squashed) VALUES(1, 1, TRUE)`)
	rt.DB.MustExec(`UPDATE orgs_org SET limits = '{"contacts": 1}' WHERE id = 1`)
	models.FlushChannelCache()
	models.FlushContactCounts()
	setup(t, rt)

	// a chat can't be started because there's no room for the visitor's contact
	code, body := post(t, rt, startURL, `{"chat_id": "`+testChatID+`"}`)
	assert.Equal(t, 422, code)
	assert.JSONEq(t, `{"message":"Error","data":[{"type":"error","error":"workspace has reached its limit of 1 contacts"}]}`, body)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM contacts_contact WHERE org_id = 1`).Returns(1)
}

// typing is posted to the service like a message, so it's tested directly with the service mocked
func TestSendEvent(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	setup(t, rt)

	ch := testChannels[0]
	testsuite.InsertChannel(t, rt, ch)

	h := newHandler(rt, channels.NewRoutes())
	assert.Equal(t, map[string]time.Duration{events.TypeTypingStarted: 5 * time.Second, events.TypeTypingStopped: 0}, h.SendableEvents(ch))

	mocks := httpx.WithMocks(rt.HTTP.Default.Transport, map[string][]*httpx.MockResponse{
		"http://chip/wi/event": {
			httpx.NewMockResponse(200, nil, []byte(`{}`)),
			httpx.NewMockResponse(200, nil, []byte(`{}`)),
			httpx.NewMockResponse(500, nil, []byte(`{"error": "error publishing event"}`)),
			httpx.MockConnectionError,
		},
	})
	rt.HTTP.Default.Transport = mocks

	channelRef := assets.NewChannelReference(assets.ChannelUUID(ch.UUID()), "WebChat")
	urn := urns.URN("webchat:" + testChatID)

	send := func(e events.Event) error {
		return h.SendEvent(ctx, ch, e, models.NewChannelLogForEventSend(ch, nil))
	}

	// typing by a user is attributed to them...
	started := events.NewTypingStarted(events.DirectionOutgoing, channelRef, urn, "")
	started.SetUser(assets.NewUserReference("c7c5c4a2-1b2c-4d3e-8f9a-0b1c2d3e4f5a", "Bob McBob"), "ui")
	require.NoError(t, send(started))

	// ...and anything else isn't
	require.NoError(t, send(events.NewTypingStopped(events.DirectionOutgoing, channelRef, urn, "")))

	reqs := mocks.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "Token sesame", reqs[0].Header.Get("Authorization"))
	body, _ := io.ReadAll(reqs[0].Body)
	assert.JSONEq(t, `{"channel_uuid": "`+channelUUID+`", "chat_id": "`+testChatID+`", "type": "typing_started", "user": {"uuid": "c7c5c4a2-1b2c-4d3e-8f9a-0b1c2d3e4f5a", "name": "Bob McBob"}}`, string(body))
	body, _ = io.ReadAll(reqs[1].Body)
	assert.JSONEq(t, `{"channel_uuid": "`+channelUUID+`", "chat_id": "`+testChatID+`", "type": "typing_stopped"}`, string(body))

	// other event types aren't sendable
	assert.EqualError(t, send(events.NewContactNameChanged("Bob")), "unsupported event type: contact_name_changed")

	// and the service refusing is an error, distinct from it being unreachable
	assert.ErrorIs(t, send(started), channels.ErrResponseStatus)
	assert.ErrorIs(t, send(started), channels.ErrConnectionFailed)

	// as is it being unconfigured
	rt.Config.ChipURL = ""
	assert.ErrorIs(t, send(started), channels.ErrChannelConfig)
}
