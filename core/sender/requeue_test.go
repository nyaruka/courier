package sender_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/core/sender"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/aws/dynamo/dyntest"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/queues"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a provider which doesn't take any message, and calls the given function when asked to
type notTakingTransport struct {
	onSend func()
}

func (t *notTakingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.onSend != nil {
		t.onSend()
	}
	return &http.Response{StatusCode: 507, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("not taken!")), Request: r}, nil
}

type requeueTest struct {
	t       *testing.T
	ctx     context.Context
	rt      *runtime.Runtime
	channel *models.Channel
}

// newRequeueTest sets up a channel whose sends go to the given provider
func newRequeueTest(t *testing.T, provider http.RoundTripper) *requeueTest {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)
	dyntest.Truncate(t, rt.Dynamo.Main.Client(), rt.Dynamo.Main.Table())

	rt.HTTP.Default.Transport = httpx.WithTraces(provider)

	channels.ActivateHandler(test.NewMockHandler(rt, &channels.Routes{}))

	channel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{})
	testsuite.InsertChannel(t, rt, channel)

	foreman := sender.NewForeman(rt, 4)
	foreman.Start()
	t.Cleanup(foreman.Stop)

	return &requeueTest{t: t, ctx: ctx, rt: rt, channel: channel}
}

// queue queues a new message for the channel, returning its UUID
func (rt *requeueTest) queue(edit func(map[string]any)) models.MsgUUID {
	m := &models.MsgOut{
		OrgID_:        1,
		UUID_:         models.MsgUUID(uuids.NewV7()),
		Contact_:      &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
		Text_:         "test",
		HighPriority_: true,
		CreatedOn_:    time.Now(),
		ChannelUUID_:  rt.channel.UUID(),
		URN_:          "tel:+12067799192",
		Origin_:       models.MsgOriginChat,
	}

	rt.rt.DB.MustExec(`INSERT INTO msgs_msg(uuid, text, high_priority, created_on, modified_on, direction, status, visibility, folder, msg_type, is_android,
		msg_count, error_count, external_identifier, channel_id, contact_id, contact_urn_id, org_id, log_uuids)
		VALUES($1, 'test', TRUE, NOW(), NOW(), 'O', 'Q', 'V', 'O', 'T', FALSE, 1, 0, '', $2, 100, 1000, 1, '{}')`, m.UUID_, rt.channel.ID())

	queued := map[string]any{}
	jsonx.MustUnmarshal(jsonx.MustMarshal(m), &queued)
	if edit != nil {
		edit(queued)
	}

	vc := rt.rt.VK.Get()
	defer vc.Close()

	require.NoError(rt.t, models.MsgQueue.Push(rt.ctx, vc, queues.OwnerID(rt.channel.UUID()), 0, true, []byte("["+string(jsonx.MustMarshal(queued))+"]")))
	return m.UUID_
}

// do runs the given command against the queue's database
func (rt *requeueTest) do(cmd string, args ...any) {
	vc := rt.rt.VK.Get()
	defer vc.Close()

	_, err := vc.Do(cmd, args...)
	assert.NoError(rt.t, err)
}

// status returns the status and error count of the given message
func (rt *requeueTest) status(c assert.TestingT, uuid models.MsgUUID) (string, int) {
	var row struct {
		Status     string `db:"status"`
		ErrorCount int    `db:"error_count"`
	}
	assert.NoError(c, rt.rt.DB.Get(&row, `SELECT status, error_count FROM msgs_msg WHERE uuid = $1`, uuid))
	return row.Status, row.ErrorCount
}

func (rt *requeueTest) assertStatus(uuid models.MsgUUID, status models.MsgStatus, errorCount int) {
	rt.t.Helper()

	assert.EventuallyWithT(rt.t, func(c *assert.CollectT) {
		s, e := rt.status(c, uuid)
		assert.Equal(c, string(status), s)
		assert.Equal(c, errorCount, e)
	}, 10*time.Second, 100*time.Millisecond)
}

// assertStaysQueued asserts that no status is written for the given message
func (rt *requeueTest) assertStaysQueued(uuid models.MsgUUID) {
	rt.t.Helper()

	assert.Never(rt.t, func() bool {
		s, e := rt.status(rt.t, uuid)
		return s != string(models.MsgStatusQueued) || e != 0
	}, 2*time.Second, 100*time.Millisecond)
}

// notTakenLogs returns the number of channel logs written for sends the provider didn't take
func (rt *requeueTest) notTakenLogs() int {
	count := 0
	for _, item := range dyntest.ScanAll(rt.t, rt.rt.Dynamo.Main.Client(), rt.rt.Dynamo.Main.Table()) {
		if strings.HasPrefix(item.Key.SK, "log#") && item.Data["type"] == "msg_send" {
			var dataGZ struct {
				HttpLogs []*httpx.Log `json:"http_logs"`
			}
			require.NoError(rt.t, dynamo.UnmarshalJSONGZ(item.DataGZ, &dataGZ))

			// a send's request is only ever logged once
			if assert.Len(rt.t, dataGZ.HttpLogs, 1) && dataGZ.HttpLogs[0].StatusCode == 507 {
				count++
			}
		}
	}
	return count
}

func (rt *requeueTest) assertNotTakenLogs(expected int) {
	rt.t.Helper()

	assert.Eventually(rt.t, func() bool { return rt.notTakenLogs() == expected }, 10*time.Second, 100*time.Millisecond)
}

// queued returns the number of messages queued and requeued for the channel
func (rt *requeueTest) queued() (int, int) {
	vc := rt.rt.VK.Get()
	defer vc.Close()

	high, bulk, requeued, err := models.MsgQueue.Size(rt.ctx, vc)
	assert.NoError(rt.t, err)
	return high + bulk, requeued
}

func TestNotSentRequeued(t *testing.T) {
	rt := newRequeueTest(t, httpx.WithMocks(nil, map[string][]*httpx.MockResponse{"http://mock.com/send": {
		httpx.NewMockResponse(507, nil, []byte(`not taken!`)),
		httpx.NewMockResponse(200, nil, []byte(`SENT`)),
	}}))

	// the message is put back, and sent once the channel's pause is over, without counting as an errored attempt
	uuid := rt.queue(nil)
	rt.assertStatus(uuid, models.MsgStatusWired, 0)

	// and the send that wasn't taken is still logged
	rt.assertNotTakenLogs(1)
}

func TestNotSentButNotPaused(t *testing.T) {
	transport := &notTakingTransport{}
	rt := newRequeueTest(t, transport)

	// break the channel's queue state whilst the message is being sent, so that sending on it can't be paused
	channelKey := "{msgs}:ch:" + string(rt.channel.UUID())
	transport.onSend = func() {
		rt.do("DEL", channelKey)
		rt.do("SET", channelKey, "not a hash")
	}

	// without a pause the message would be popped again straight away, so it's errored like any other attempt
	uuid := rt.queue(nil)
	rt.assertStatus(uuid, models.MsgStatusErrored, 1)
	rt.assertNotTakenLogs(1)

	queued, requeued := rt.queued()
	assert.Zero(t, queued)
	assert.Zero(t, requeued)
}

func TestNotSentButNotRequeued(t *testing.T) {
	transport := &notTakingTransport{}
	rt := newRequeueTest(t, transport)

	// break the channel's requeued messages whilst the message is being sent, so that it can't be put back
	requeueKey := "{msgs}:q:" + string(rt.channel.UUID()) + "/2"
	transport.onSend = func() { rt.do("SET", requeueKey, "not a sorted set") }

	uuid := rt.queue(nil)
	rt.assertNotTakenLogs(1)
	rt.do("DEL", requeueKey)

	// the requeue may have happened despite the error, so the message isn't errored as well, as that would have it
	// retried from both places
	rt.assertStaysQueued(uuid)
	assert.Equal(t, 1, rt.notTakenLogs())
}

func TestNotSentTooManyTimes(t *testing.T) {
	rt := newRequeueTest(t, &notTakingTransport{})

	// a message that has one requeue left is put back once more, and then errored
	uuid := rt.queue(func(m map[string]any) { m["requeues"] = sender.MaxRequeues - 1 })
	rt.assertStatus(uuid, models.MsgStatusErrored, 1)
	rt.assertNotTakenLogs(2)

	queued, requeued := rt.queued()
	assert.Zero(t, queued)
	assert.Zero(t, requeued)
}
