package sender_test

import (
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/core/sender"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/queues"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForemanStopWhilstBusy(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	const numMsgs = 300

	sends := make([]*httpx.MockResponse, numMsgs)
	for i := range sends {
		sends[i] = httpx.NewMockResponse(200, nil, []byte(`SENT`))
	}
	rt.HTTP.Default.Transport = httpx.WithTraces(httpx.WithMocks(nil, map[string][]*httpx.MockResponse{"http://mock.com/send": sends}))

	// activate the mock handler, which a server would otherwise do on startup
	channels.ActivateHandler(test.NewMockHandler(rt, &channels.Routes{}))

	channel := test.NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230", "MCK", "2021", "US", []string{urns.Phone.Prefix}, map[string]any{})
	testsuite.InsertChannel(t, rt, channel)

	vc := rt.VK.Get()
	defer vc.Close()

	// queue each message as its own batch so that they can all be popped straight away
	for range numMsgs {
		m := &models.MsgOut{
			OrgID_:        1,
			UUID_:         models.MsgUUID(uuids.NewV7()),
			Contact_:      &models.ContactReference{ID: 100, UUID: "a984069d-0008-4d8c-a772-b14a8a6acccc"},
			Text_:         "test",
			HighPriority_: true,
			CreatedOn_:    time.Now(),
			ChannelUUID_:  channel.UUID(),
			URN_:          "tel:+12067799192",
			Origin_:       models.MsgOriginChat,
		}

		rt.DB.MustExec(`INSERT INTO msgs_msg(uuid, text, high_priority, created_on, modified_on, direction, status, visibility, folder, msg_type, is_android,
			msg_count, error_count, external_identifier, channel_id, contact_id, contact_urn_id, org_id, log_uuids)
			VALUES($1, 'test', TRUE, NOW(), NOW(), 'O', 'Q', 'V', 'O', 'T', FALSE, 1, 0, '', $2, 100, 1000, 1, '{}')`, m.UUID_, channel.ID())

		require.NoError(t, models.MsgQueue.Push(ctx, vc, queues.OwnerID(channel.UUID()), 0, true, []byte("["+string(jsonx.MustMarshal(m))+"]")))
	}

	// repeatedly stop a foreman that still has messages to assign - a message popped once the senders have been
	// stopped has nobody to send it, so stopping has to stop the popping first
	for range 20 {
		foreman := sender.NewForeman(rt, 4)
		foreman.Start()
		time.Sleep(5 * time.Millisecond)
		foreman.Stop()
	}

	// let a final foreman send whatever is left
	foreman := sender.NewForeman(rt, 4)
	foreman.Start()
	defer foreman.Stop()

	// every message should have been sent, i.e. none was popped by a stopping foreman and dropped
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var unsent int
		require.NoError(c, rt.DB.Get(&unsent, `SELECT count(*) FROM msgs_msg WHERE channel_id = $1 AND status != 'W'`, channel.ID()))
		assert.Zero(c, unsent, "messages left unsent")
	}, 20*time.Second, 100*time.Millisecond)
}
