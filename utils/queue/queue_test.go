package queue_test

import (
	"fmt"
	"sync"
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

func TestLegacyLua(t *testing.T) {
	rp := getPool(t)
	rc := rp.Get()
	defer rc.Close()

	// start our dethrottler
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	queue.StartDethrottler(rp, quitter, wg)
	defer close(quitter)

	rate := 10

	// add 20 messages with ids 0-19
	for i := range 20 {
		err := queue.PushOntoLegacyQueue(rc, "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority)
		require.NoError(t, err)
	}

	// align ourselves to mid-second so that our operations don't race the dethrottler (which fires just after
	// each second boundary) or second boundaries themselves
	delay := time.Second*2 - time.Duration(time.Now().UnixNano()%int64(time.Second)) + time.Millisecond*500
	time.Sleep(delay)

	// pop 10 items off
	for i := 0; i < 10; i++ {
		q, value, err := queue.PopFromQueue(rc)
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)
	}

	// next value should be throttled
	q, value, err := queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, "", value)
	assert.Equal(t, queue.Retry, q)

	// check our redis state
	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 10})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{})

	// adding more items shouldn't change that
	err = queue.PushOntoLegacyQueue(rc, "chan1", rate, `[{"id":30}]`, queue.LowPriority)
	assert.NoError(t, err)

	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 10})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{})

	// but if we wait, our next msg should be our highest priority
	time.Sleep(time.Second)

	err = queue.PushOntoLegacyQueue(rc, "chan1", rate, `[{"id":31}]`, queue.HighPriority)
	assert.NoError(t, err)

	q, value, err = queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":31}`, value)

	// should get next five bulk msgs fine
	for i := 10; i < 15; i++ {
		q, value, err := queue.PopFromQueue(rc)
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)

	}

	// push a multi-message batch for a single contact
	err = queue.PushOntoLegacyQueue(rc, "chan1", rate, `[{"id":32}, {"id":33}]`, queue.HighPriority)
	assert.NoError(t, err)

	q, value, err = queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":32}`, value)

	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{"msgs:chan1|10": 17})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{"msgs:chan1|10": 0})

	// sleep a few seconds
	time.Sleep(2 * time.Second)

	// pop remaining bulk off
	for i := 15; i < 20; i++ {
		q, value, err := queue.PopFromQueue(rc)
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)
	}

	// next should be 30
	q, value, err = queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.NotEqual(t, q, queue.EmptyQueue)
	assert.Equal(t, `{"id":30}`, value)

	// popping again should give us nothing since it is too soon to send 33
	q = queue.Retry
	for q == queue.Retry {
		q, value, err = queue.PopFromQueue(rc)
	}
	assert.NoError(t, err)
	assert.Equal(t, queue.EmptyQueue, q)
	assert.Empty(t, value)

	// but if we sleep 6 seconds should get it
	time.Sleep(time.Second * 6)

	q, value, err = queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":33}`, value)

	// nothing should be left
	q = queue.Retry
	for q == queue.Retry {
		q, value, err = queue.PopFromQueue(rc)
	}
	assert.NoError(t, err)
	assert.Equal(t, queue.EmptyQueue, q)
	assert.Empty(t, value)

}

func TestLegacyTPSCost(t *testing.T) {
	rp := getPool(t)
	rc := rp.Get()
	defer rc.Close()

	rate := 10

	// push a message whose tps_cost consumes the entire rate, followed by a regular message
	err := queue.PushOntoLegacyQueue(rc, "chan1", rate, `[{"id":0,"tps_cost":10}]`, queue.HighPriority)
	require.NoError(t, err)
	err = queue.PushOntoLegacyQueue(rc, "chan1", rate, `[{"id":1}]`, queue.HighPriority)
	require.NoError(t, err)

	// align ourselves to mid-second so our pops and TPS key reads land in the same second
	delay := time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second)) + time.Millisecond*500
	time.Sleep(delay)

	q, value, err := queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":0,"tps_cost":10}`, value)

	// popped message should have been charged its full cost against this second's TPS key
	tpsUsed, err := redis.Int(rc.Do("GET", fmt.Sprintf("msgs:chan1|10:tps:%d", time.Now().Unix())))
	assert.NoError(t, err)
	assert.Equal(t, 10, tpsUsed)

	// and against the channel's usage shared with the new layout
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "used", "10")

	// so the next pop should be throttled even though only one message was popped
	q, value, err = queue.PopFromQueue(rc)
	assert.NoError(t, err)
	assert.Equal(t, queue.Retry, q)
	assert.Equal(t, "", value)

	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 1})
}

func TestLegacyThrottle(t *testing.T) {
	assert := assert.New(t)
	pool := getPool(t)
	conn := pool.Get()
	defer conn.Close()

	// start our dethrottler
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	queue.StartDethrottler(pool, quitter, wg)

	insertCount := 30
	rate := 10

	// insert items with our set limit
	for i := range insertCount {
		err := queue.PushOntoLegacyQueue(conn, "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.HighPriority)
		assert.NoError(err)
		time.Sleep(1 * time.Microsecond)
	}

	// start timing
	start := time.Now()
	curr := 0
	var task queue.WorkerToken
	var err error
	var value string
	for curr < insertCount {
		task, value, err = queue.PopFromQueue(conn)
		assert.NoError(err)

		// if this wasn't throttled
		if value != "" {
			expected := fmt.Sprintf(`{"id":%d}`, curr)
			assert.Equal(expected, value, "Out of order msg")
			curr++

			err = queue.MarkComplete(conn, task)
			assert.NoError(err)
		} else {
			// otherwise sleep a bit
			time.Sleep(100 * time.Millisecond)
		}
	}

	// if we haven't seen all messages, fail
	assert.Equal(insertCount, curr, "Did not read all messages")

	// if this took less than 1 second or more than 3 seconds, fail, should have throttled
	expected := time.Duration((insertCount / rate) - 2)
	elapsed := time.Since(start)
	if elapsed < expected*time.Second || elapsed > (expected+2)*time.Second {
		t.Errorf("Did not throttle properly, took: %f", elapsed.Seconds())
	}

	// close our dethrottler
	close(quitter)
	wg.Wait()
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
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "5")

	// once nothing is in flight, the channel's state expires
	for range 6 {
		require.NoError(t, queue.MarkComplete(rc, "chan1"))
	}
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "0")

	ttl, err := redis.Int(rc.Do("TTL", "{msgs}:ch:chan1"))
	assert.NoError(t, err)
	assert.Greater(t, ttl, 0)

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

func TestQueueBothLayouts(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	// a channel with items in both layouts, and another with items only in the legacy layout
	require.NoError(t, queue.PushOntoLegacyQueue(rc, "chan1", 3, `[{"id":1}]`, queue.HighPriority))
	require.NoError(t, queue.PushOntoLegacyQueue(rc, "chan1", 3, `[{"id":2}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoLegacyQueue(rc, "chan2", 0, `[{"id":3}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 3, `[{"id":4}]`, queue.LowPriority))
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 3, `[{"id":5}]`, queue.LowPriority))

	high, bulk, err := queue.Size(rc)
	assert.NoError(t, err)
	assert.Equal(t, 1, high)
	assert.Equal(t, 4, bulk)

	waitForMidSecond(t, rc)

	// chan1's TPS is shared between layouts, so we only get 3 of its 4 items this second
	tokens := map[queue.WorkerToken]int{}
	values := []string{}
	for {
		token, value := popNext(t, rc)
		if token == queue.EmptyQueue {
			break
		}
		tokens[token]++
		values = append(values, value)
	}

	assert.Len(t, values, 4)
	assert.Equal(t, 1, tokens["msgs:chan2|0"])
	assert.Equal(t, 3, tokens["msgs:chan1|3"]+tokens["chan1"])

	for token, n := range tokens {
		for range n {
			require.NoError(t, queue.MarkComplete(rc, token))
		}
	}

	// nothing left in flight in either layout
	for _, key := range []string{"msgs:active", "msgs:throttled"} {
		n, err := redis.Int(rc.Do("ZCOUNT", key, "(0", "+inf"))
		assert.NoError(t, err)
		assert.Equal(t, 0, n, "workers in %s", key)
	}
	assertvk.HGet(t, rc, "{msgs}:ch:chan1", "inflight", "0")

	// and the rest the next second
	time.Sleep(time.Second)

	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	queue.StartDethrottler(getPool(t), quitter, wg)
	defer close(quitter)

	time.Sleep(1100 * time.Millisecond)

	token, value := popNext(t, rc)
	assert.NotEqual(t, queue.EmptyQueue, token)
	values = append(values, value)

	token, _ = popNext(t, rc)
	assert.Equal(t, queue.EmptyQueue, token)

	assert.ElementsMatch(t, []string{`{"id":1}`, `{"id":2}`, `{"id":3}`, `{"id":4}`, `{"id":5}`}, values)
}

func TestQueuePushWhenThrottled(t *testing.T) {
	rc := getPool(t).Get()
	defer rc.Close()

	waitForMidSecond(t, rc)

	// use this second's TPS from the legacy layout
	require.NoError(t, queue.PushOntoLegacyQueue(rc, "chan1", 1, `[{"id":1}]`, queue.HighPriority))
	_, value := popNext(t, rc)
	assert.Equal(t, `{"id":1}`, value)

	// so an item pushed now waits until the next second
	require.NoError(t, queue.PushOntoQueue(rc, "chan1", 1, `[{"id":2}]`, queue.HighPriority))
	assertvk.ZGetAll(t, rc, "{msgs}:ready", map[string]float64{})
	assertvk.ZGetAll(t, rc, "{msgs}:waiting", map[string]float64{"chan1": serverSecond(t, rc) + 1})

	time.Sleep(time.Second)

	_, value = popNext(t, rc)
	assert.Equal(t, `{"id":2}`, value)
}
