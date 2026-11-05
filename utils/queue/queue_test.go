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

func getPool(t *testing.T) *redis.Pool {
	redisPool := &redis.Pool{
		Wait:        true,                            // makes callers wait for a connection
		MaxActive:   5,                               // only open this many concurrent connections at once
		MaxIdle:     2,                               // only keep up to 2 idle
		IdleTimeout: 240 * time.Second,               // how long to wait before reaping a connection
		Dial:        assertvk.ClaimDB(t).Pool().Dial, // this test's own database - see setup_test.go
	}
	t.Cleanup(func() { redisPool.Close() }) // before the database is released

	return redisPool
}

// sleeps until the middle of the server's next second so that what follows happens within one second
func waitForMidSecond(t *testing.T, rc redis.Conn) {
	vals, err := redis.Int64s(rc.Do("TIME"))
	require.NoError(t, err)
	time.Sleep(time.Second + 500*time.Millisecond - time.Duration(vals[1])*time.Microsecond)
}

func serverSecond(t *testing.T, rc redis.Conn) float64 {
	vals, err := redis.Int64s(rc.Do("TIME"))
	require.NoError(t, err)
	return float64(vals[0])
}

// pops until a value or an empty queue
func popNext(t *testing.T, rc redis.Conn) (queue.WorkerToken, string) {
	for {
		token, value, err := queue.PopFromQueue(rc)
		require.NoError(t, err)
		if token != queue.Retry {
			return token, value
		}
	}
}

func TestQueue(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	for i := range 3 {
		require.NoError(t, queue.PushOntoQueue(rc, "chan1", 2, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority))
	}
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 2, `[{"id":10}]`, queue.HighPriority))

	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 0})
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{})
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/1", 1)
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/0", 3)
	assertvk.HGetAll(t, rc, "{msgs}:ch:chan1", map[string]string{"tps": "2"})

	waitForMidSecond(t, rc)

	// high priority first, then bulk
	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":10}`, value)

	token, value = popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":0}`, value)

	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 2})
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "2")
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "used", "2")

	// channel has used this second's TPS so waits until the next
	token, value, err := queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.Retry, token)
	assert.Equal(t, "", value)

	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{"chan1": serverSecond(t, rc) + 1})

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	// pushing doesn't wake it
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 2, `[{"id":3}]`, queue.LowPriority))
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})

	// completing sends updates its count of sends in flight even while it's waiting
	require.NoError(t, queue.MarkComplete(rc, "chan1"))
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "1")

	// next second it's woken with that count
	time.Sleep(time.Second)

	token, value = popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":1}`, value)
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 2})

	// a new TPS applies to everything already queued
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":4}]`, queue.LowPriority))

	for _, expected := range []string{`{"id":2}`, `{"id":3}`, `{"id":4}`} {
		_, value = popNext(t, rc)
		assert.Equal(t, expected, value)
	}

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})

	// with nothing queued, the channel's sends in flight are forgotten and its state expires
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "0")

	ttl, err := redis.Int(rc.Do("TTL", "{msgs}:ch:chan1"))
	assert.NoError(t, err)
	assert.Greater(t, ttl, 0)

	// so sends that complete afterwards don't take it below zero
	for range 5 {
		require.NoError(t, queue.MarkComplete(rc, "chan1"))
	}
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "0")

	// until something is pushed
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 2, `[{"id":5}]`, queue.LowPriority))

	ttl, err = redis.Int(rc.Do("TTL", "{msgs}:ch:chan1"))
	assert.NoError(t, err)
	assert.Equal(t, -1, ttl)
}

func TestQueueBatches(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 10, `[{"id":1},{"id":2}]`, queue.LowPriority))

	_, value := popNext(t, rc)
	assert.Equal(t, `{"id":1}`, value)

	// rest of the batch is scheduled a few seconds out, so the channel waits for it
	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})
	assertvk.ZCard(t, rc, "{msgs}:waiting", 1)
	assertvk.ZCard(t, rc, "{msgs}:q:chan1/1", 1)

	// but a new item doesn't have to wait
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 10, `[{"id":3}]`, queue.HighPriority))
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 1})
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{})

	_, value = popNext(t, rc)
	assert.Equal(t, `{"id":3}`, value)

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	time.Sleep(3 * time.Second)

	_, value = popNext(t, rc)
	assert.Equal(t, `{"id":2}`, value)
}

func TestQueueFairness(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	for i := range 3 {
		require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority))
		require.NoError(t, queue.PushOntoQueue(rc, "chan2", 0, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority))
	}

	// the channel with the fewest sends in flight goes next
	for _, expected := range []string{"chan1", "chan2", "chan1", "chan2"} {
		token, _ := popNext(t, rc)
		assert.Equal(t, queue.WorkerToken(expected), token)
	}

	require.NoError(t, queue.MarkComplete(rc, "chan2"))
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 2, "chan2": 1})

	token, _ := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan2"), token)
}

func TestQueueTPSCost(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	// a message whose tps_cost consumes the entire rate, followed by a regular message
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 10, `[{"id":0,"tps_cost":10}]`, queue.HighPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 10, `[{"id":1}]`, queue.HighPriority))

	waitForMidSecond(t, rc)

	token, value := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assert.Equal(t, `{"id":0,"tps_cost":10}`, value)
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "used", "10")

	// so the next pop waits until the next second even though only one message was popped
	token, value, err := queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.Retry, token)
	assert.Equal(t, "", value)
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{"chan1": serverSecond(t, rc) + 1})

	time.Sleep(time.Second)

	_, value = popNext(t, rc)
	assert.Equal(t, `{"id":1}`, value)
}

func TestQueueThrottle(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	insertCount := 30
	rate := 10

	for i := range insertCount {
		require.NoError(t, queue.PushOntoQueue(rc, "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.HighPriority))
	}

	waitForMidSecond(t, rc)
	start := time.Now()

	for curr := 0; curr < insertCount; {
		token, value, err := queue.PopFromQueue(rc)
		require.NoError(t, err)

		if value != "" {
			assert.Equal(t, fmt.Sprintf(`{"id":%d}`, curr), value, "out of order msg")
			curr++
			require.NoError(t, queue.MarkComplete(rc, token))
		} else if token == queue.EmptyQueue {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 10 a second starting mid-second, so the last 10 are popped at the start of the third second
	elapsed := time.Since(start)
	assert.Greater(t, elapsed, 1400*time.Millisecond)
	assert.Less(t, elapsed, 2500*time.Millisecond)
}

func TestQueueSize(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	high, bulk, err := queue.Size(rc)
	assert.NoError(t, err)
	assert.Equal(t, 0, high)
	assert.Equal(t, 0, bulk)

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 1, `[{"id":1}]`, queue.HighPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 1, `[{"id":2}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 1, `[{"id":3}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan2", 0, `[{"id":4}]`, queue.LowPriority))

	high, bulk, err = queue.Size(rc)
	assert.NoError(t, err)
	assert.Equal(t, 1, high)
	assert.Equal(t, 3, bulk)

	waitForMidSecond(t, rc)

	// pop until chan1 has used this second's TPS and is waiting, as its remaining items still count
	popNext(t, rc)
	popNext(t, rc)
	token, _ := popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})
	assertvk.ZCard(t, rc, "{msgs}:waiting", 1)

	high, bulk, err = queue.Size(rc)
	assert.NoError(t, err)
	assert.Equal(t, 0, high)
	assert.Equal(t, 2, bulk)
}

func TestQueueForgetsLostSends(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":1}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan2", 0, `[{"id":2}]`, queue.LowPriority))

	// pop from chan1 and never mark it complete, as if the worker died
	token, _ := popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan1"), token)
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 1, "chan2": 0})

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.WorkerToken("chan2"), token)
	require.NoError(t, queue.MarkComplete(rc, "chan2"))

	// once chan1 is found drained, the lost send no longer counts against it
	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 0, `[{"id":3}]`, queue.LowPriority))
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{"chan1": 0})
}
