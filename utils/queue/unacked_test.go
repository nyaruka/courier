package queue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
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
	require.NoError(t, queue.Acked(rc, "chan1", "msg1", 2, 0))
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
		require.NoError(t, queue.Acked(rc, "chan1", fmt.Sprintf("msg%d", i), 0, 30*time.Second))
	}

	// repeated reports don't count again
	require.NoError(t, queue.Acked(rc, "chan1", "msg0", 0, 30*time.Second))
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
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg2", 1, 0))

	// pretend those sends were so long ago that their acknowledgements must have been lost - one long enough ago that
	// it's no longer kept even for a late acknowledgement
	now := serverSecond(t, rc)
	_, err := rc.Do("ZADD", "{msgs}:unacked:chan1", now-queue.UnackedTTL.Seconds()-1, "msg1", now-(2*time.Hour).Seconds(), "msg2")
	require.NoError(t, err)

	// neither counts against the limit any more, but the more recent is kept in case its acknowledgement is just late
	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	assertvk.ZRange(t, rc, "{msgs}:unacked:chan1", 0, -1, []string{"msg1"})

	// the keys expire once the channel stops sending
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg3", 1, 0))
	for _, key := range []string{"{msgs}:unacked:chan1", "{msgs}:acklimits:chan1"} {
		ttl, err := redis.Int(rc.Do("TTL", key))
		require.NoError(t, err)
		assert.Greater(t, ttl, 0, "no TTL on %s", key)
	}
}

func TestQueueMaxUnackedCountsInflight(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	for i := range 3 {
		require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority))
	}
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg1", 2, 0))

	// one send is in flight and not yet tracked, which with the one tracked puts chan1 at its limit
	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":0}`, value)

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/0", 2)
}

func TestQueueForgetsOldUnackedForLag(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))

	// a backlog that will never be acknowledged, e.g. because Kannel restarted and lost its delivery reports
	for i := range 200 {
		require.NoError(t, queue.TrackUnacked(rc, "chan1", fmt.Sprintf("msg%d", i), 0, 10*time.Second))
	}

	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	// once it's older than two lags it no longer holds the channel back, though it's still kept for the count limit
	now := serverSecond(t, rc)
	for i := range 200 {
		_, err := rc.Do("ZADD", "{msgs}:unacked:chan1", now-21, fmt.Sprintf("msg%d", i))
		require.NoError(t, err)
	}

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 200)
}

func TestQueueAckedAppliesLimits(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg1", 2, 0))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg2", 2, 0))

	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	// an acknowledgement of a send we weren't tracking still applies the channel's limits, here raising them
	require.NoError(t, queue.Acked(rc, "chan1", "msg9", 5, 0))
	assertvk.HGetAll(t, rc, "{msgs}:acklimits:chan1", map[string]string{"max_unacked": "5", "max_ack_lag": "0"})
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 2)

	// and zero limits remove them
	require.NoError(t, queue.Acked(rc, "chan1", "msg1", 0, 0))
	assertvk.NotExists(t, rc, "{msgs}:acklimits:chan1")
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 1)

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
}

func TestQueueAckRate(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	for i := range 150 {
		require.NoError(t, queue.TrackUnacked(rc, "chan1", fmt.Sprintf("msg%d", i), 0, 10*time.Second))
	}

	waitForMidSecond(t, rc)
	now := int(serverSecond(t, rc))
	slot := now % 11

	// acknowledgements from before the window, left in the ring of slots, don't count towards the rate
	_, err := rc.Do("HSET", "{msgs}:acks:chan1", fmt.Sprintf("s%d", (slot+1)%11), now-11, fmt.Sprintf("c%d", (slot+1)%11), 1000)
	require.NoError(t, err)

	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	// and when the slot comes round again it starts over
	_, err = rc.Do("HSET", "{msgs}:acks:chan1", fmt.Sprintf("s%d", slot), now-11, fmt.Sprintf("c%d", slot), 1000)
	require.NoError(t, err)
	require.NoError(t, queue.Acked(rc, "chan1", "msg0", 0, 10*time.Second))
	assertvk.HGet(t, rc, "{msgs}:acks:chan1", fmt.Sprintf("s%d", slot), fmt.Sprint(now))
	assertvk.HGet(t, rc, "{msgs}:acks:chan1", fmt.Sprintf("c%d", slot), "1")

	// a late acknowledgement, of a send that no longer counts against the limits, still counts towards the rate
	_, err = rc.Do("ZADD", "{msgs}:unacked:chan1", float64(now)-queue.UnackedTTL.Seconds()-60, "msg1")
	require.NoError(t, err)
	require.NoError(t, queue.Acked(rc, "chan1", "msg1", 0, 10*time.Second))
	assertvk.HGet(t, rc, "{msgs}:acks:chan1", fmt.Sprintf("c%d", slot), "2")

	// a send that's untracked doesn't count
	require.NoError(t, queue.Untrack(rc, "chan1", "msg2"))
	require.NoError(t, queue.Acked(rc, "chan1", "msg2", 0, 10*time.Second))
	assertvk.HGet(t, rc, "{msgs}:acks:chan1", fmt.Sprintf("c%d", slot), "2")
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 147)
}

func TestQueueAckLimitsLapse(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	require.NoError(t, queue.TrackUnacked(rc, "chan1", "msg1", 1, 0))

	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	// the limits only last a minute unless a send or an acknowledgement refreshes them, unlike the sends themselves
	limitsTTL, err := redis.Int(rc.Do("TTL", "{msgs}:acklimits:chan1"))
	require.NoError(t, err)
	assert.True(t, limitsTTL > 0 && limitsTTL <= 60, "unexpected limits TTL %d", limitsTTL)

	sendsTTL, err := redis.Int(rc.Do("TTL", "{msgs}:unacked:chan1"))
	require.NoError(t, err)
	assert.Greater(t, sendsTTL, 60)

	// so a channel that stops getting delivery reports, e.g. after they're turned off, isn't held back by them for long
	_, err = rc.Do("PEXPIRE", "{msgs}:acklimits:chan1", 1)
	require.NoError(t, err)

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	assertvk.ZCard(t, rc, "{msgs}:unacked:chan1", 1)
}
