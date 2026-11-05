package queue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/utils/queue"
	"github.com/nyaruka/vkutil/assertvk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueMaxUnacked(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	for i := range 2 {
		require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority))
		require.NoError(t, queue.PushOntoQueue(rc, "chan2", 0, fmt.Sprintf(`[{"id":%d}]`, 10+i), queue.LowPriority))
	}

	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg1", 2, 0))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg2", 2, 0))

	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 2)
	assertvk.HGetAll(t, rc, "{msgs}:acklimits:chan1", map[string]string{"max_unacked": "2", "max_ack_lag": "0"})

	waitForMidSecond(t, rc)

	// chan1 is at its limit so waits, while chan2 still pops
	for i := range 2 {
		token, value := popNext(t, rc)
		assert.Equal(t, queue.WorkerToken("chan2"), token)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, 10+i), value)
		require.NoError(t, queue.MarkComplete(rc, token))
	}
	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{"chan1": serverSecond(t, rc) + 1})
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/0", 2)

	// an acknowledgement frees a slot, so once its wait is over chan1 pops one more and is at its limit again
	require.NoError(t, queue.Acked(rc, "chan1", "msg1"))
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 1)

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":0}`, value)
	require.NoError(t, queue.MarkComplete(rc, token))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg3", 2, 0))

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/0", 1)
}

func TestQueueMaxAckLag(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan2", 0, `[{"id":2}]`, queue.LowPriority))

	// below the floor the lag doesn't apply, even with nothing acknowledged yet
	for i := range 99 {
		require.NoError(t, queue.TrackUnacked(rc, "chan2", fmt.Sprintf("msg%d", i), 0, 30*time.Second))
	}

	// a backlog that nothing has acknowledged recently would never clear, so chan1 waits
	for i := range 300 {
		require.NoError(t, queue.TrackUnacked(rc, "chan1", fmt.Sprintf("msg%d", i), 0, 30*time.Second))
	}

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan2"), token)
	assert.Equal(t, `{"id":2}`, value)
	require.NoError(t, queue.MarkComplete(rc, token))

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{"chan1": serverSecond(t, rc) + 1})

	// acknowledging 100 in a second is a rate of 10/s over the window, so the 200 left would take 20s to clear
	for i := range 100 {
		require.NoError(t, queue.Acked(rc, "chan1", fmt.Sprintf("msg%d", i)))
	}

	// repeated reports don't count again
	require.NoError(t, queue.Acked(rc, "chan1", "msg0"))
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 200)

	// the second they arrived in only counts once it's over, then 20s is within the max of 30s so chan1 pops
	waitForMidSecond(t, rc)

	token, value = popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	require.NoError(t, queue.MarkComplete(rc, token))

	// but over a max of 10s it waits
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":3}]`, queue.LowPriority))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg300", 0, 10*time.Second))

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/0", 1)
}

func TestQueueForgetsLostAcks(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg1", 1, 0))

	// pretend that send was so long ago that its acknowledgement must have been lost
	now := serverSecond(t, rc)
	_, err := rc.Do("ZADD", "{msgs}:unacked:chan1", now-queue.UnackedTTL.Seconds()-1, "msg1")
	require.NoError(t, err)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 0)

	// both keys expire once the channel stops sending
	ttl, err := rc.Do("TTL", "{msgs}:acklimits:chan1")
	require.NoError(t, err)
	assert.Greater(t, ttl, int64(0))
}
