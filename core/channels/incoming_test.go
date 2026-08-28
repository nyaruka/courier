package channels_test

import (
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/courier/v26/testsuite"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncomingResponse(t *testing.T) {
	ch := test.NewMockChannel("dbc126ed-66bc-4e28-b67b-81dc3327c95d", "KN", "2020", "US", []string{urns.Phone.Prefix}, nil)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, ch, nil, nil)

	msg := models.NewIncomingMsg(ch, "tel:+12065551212", "hello", "ext1", clog)
	status := models.NewStatusUpdateByExternalID(ch, "ext2", models.MsgStatusDelivered, clog)
	event := models.NewChannelEvent(ch, models.EventTypeStopContact, "tel:+12065551212", clog)

	// a response describes each part of the request in the order the handler added it, including the parts
	// that weren't written
	w := httptest.NewRecorder()
	err := channels.WriteIncomingResponse(w, []channels.IncomingResult{
		{Event: msg, Outcome: channels.OutcomeWritten},
		{Event: status, Outcome: channels.OutcomeWritten},
		{Event: event, Outcome: channels.OutcomeWritten},
		{Details: "ignoring echo", Outcome: channels.OutcomeIgnored},
	})
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"message":"Events Handled"`)
	assert.Contains(t, body, `"type":"msg"`)
	assert.Contains(t, body, `"type":"status"`)
	assert.Contains(t, body, `"type":"event"`)
	assert.Contains(t, body, `{"type":"info","info":"ignoring echo"}`)

	// an empty set of items still gets a well formed response
	w = httptest.NewRecorder()
	assert.NoError(t, channels.WriteIncomingResponse(w, nil))
	assert.Equal(t, "{\"message\":\"Events Handled\",\"data\":[]}\n", w.Body.String())
}

func TestWriteIncoming(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.ResetDB(t, rt)
	testsuite.ResetValkey(t, rt)

	ch, err := models.GetChannel(ctx, "KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	require.NoError(t, err)
	clog := models.NewChannelLog(models.ChannelLogTypeUnknown, ch, nil, nil)

	// a mixed batch is written in the order it was added, and each item reports what became of it
	in := channels.NewIncoming()
	in.Msg(models.NewIncomingMsg(ch, "tel:+12065551212", "hello", "ext1", clog))
	in.Ignored("ignoring echo")
	in.Status(models.NewStatusUpdateByExternalID(ch, "ext2", models.MsgStatusDelivered, clog))
	in.Event(models.NewChannelEvent(ch, models.EventTypeStopContact, "tel:+12065551313", clog))
	assert.Equal(t, 4, in.Len())

	results, err := channels.WriteIncoming(ctx, rt, in, clog)
	assert.NoError(t, err)
	require.Len(t, results, 4)
	assert.Equal(t, channels.OutcomeWritten, results[0].Outcome)
	assert.Equal(t, channels.OutcomeIgnored, results[1].Outcome)
	assert.Equal(t, "ignoring echo", results[1].Details)
	assert.Equal(t, channels.OutcomeWritten, results[2].Outcome)
	assert.Equal(t, channels.OutcomeWritten, results[3].Outcome)

	// the ignored item isn't an event because nothing was written for it
	assert.Len(t, channels.IncomingEvents(results), 3)

	// and the things that reported themselves as written really were - a database failure would have been
	// absorbed by the spool and still reported as written, so check the rows rather than trusting the outcome
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM msgs_msg WHERE text = 'hello' AND external_identifier = 'ext1'`).Returns(1)
	assertdb.Query(t, rt.DB, `SELECT count(*) FROM channels_channelevent WHERE event_type = 'stop_contact'`).Returns(1)

	// a message we've already received is reported as a duplicate rather than written again
	in = channels.NewIncoming()
	in.Msg(models.NewIncomingMsg(ch, "tel:+12065551212", "hello", "ext1", clog))

	results, err = channels.WriteIncoming(ctx, rt, in, clog)
	assert.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, channels.OutcomeDuplicate, results[0].Outcome)

	// but it's still an event, because it's something we received and recognized
	assert.Len(t, channels.IncomingEvents(results), 1)

	// writing stops at the first item that fails, so the results describe the prefix we got through
	in = channels.NewIncoming()
	in.Msg(models.NewIncomingMsg(ch, "tel:+12065551212", "first", "ext3", clog))
	in.Msg(models.NewIncomingMsg(ch, "tel:+12065551212", "second", "ext4", clog).WithAttachment("data:....."))
	in.Msg(models.NewIncomingMsg(ch, "tel:+12065551212", "third", "ext5", clog))

	results, err = channels.WriteIncoming(ctx, rt, in, clog)
	assert.EqualError(t, err, "unable to decode attachment data: illegal base64 data at input byte 0")
	require.Len(t, results, 2)
	assert.Equal(t, channels.OutcomeWritten, results[0].Outcome)
	assert.Equal(t, channels.OutcomeFailed, results[1].Outcome)
	assert.Error(t, results[1].Err)

	// the item that failed isn't an event, but the one written before it is
	events := channels.IncomingEvents(results)
	require.Len(t, events, 1)
	assert.Equal(t, "first", events[0].(*models.MsgIn).Text())
}
