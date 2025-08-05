package queue_test

import (
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier/utils/queue"
	"github.com/nyaruka/vkutil/assertvk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPool() *redis.Pool {
	redisPool := &redis.Pool{
		Wait:        true,              // makes callers wait for a connection
		MaxActive:   5,                 // only open this many concurrent connections at once
		MaxIdle:     2,                 // only keep up to 2 idle
		IdleTimeout: 240 * time.Second, // how long to wait before reaping a connection
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", "localhost:6379")
			if err != nil {
				return nil, err
			}
			_, err = conn.Do("SELECT", 0)
			return conn, err
		},
	}
	conn := redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("FLUSHDB")
	if err != nil {
		log.Fatal(err)
	}

	return redisPool
}

func TestLua(t *testing.T) {
	rp := getPool()
	rc := rp.Get()
	defer rc.Close()

	// start our dethrottler
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	queue.StartDethrottler(rp, quitter, wg, "msgs")
	defer close(quitter)

	rate := 10

	// add 20 messages with ids 0-19
	for i := range 20 {
		err := queue.PushOntoQueue(rc, "msgs", "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority)
		require.NoError(t, err)
	}

	// get ourselves aligned with a second boundary
	delay := time.Second*2 - time.Duration(time.Now().UnixNano()%int64(time.Second))
	time.Sleep(delay)

	// mark chan1 as rate limited
	rc.Do("SET", "rate_limit_bulk:chan1", "engaged")
	rc.Do("EXPIRE", "rate_limit_bulk:chan1", 5)

	// popping shouldn't error or return a value
	q, value, err := queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.Equal(t, "", value)
	assert.Equal(t, queue.Retry, q)

	// unmark chan1 as rate limited
	rc.Do("DEL", "rate_limit_bulk:chan1")

	// pop 10 items off
	for i := 0; i < 10; i++ {
		q, value, err := queue.PopFromQueue(rc, "msgs")
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)
	}

	// next value should be throttled
	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.Equal(t, "", value)
	assert.Equal(t, queue.Retry, q)

	// check our redis state
	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 10})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{})

	// adding more items shouldn't change that
	err = queue.PushOntoQueue(rc, "msgs", "chan1", rate, `[{"id":30}]`, queue.LowPriority)
	assert.NoError(t, err)

	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 10})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{})

	// but if we wait, our next msg should be our highest priority
	time.Sleep(time.Second)

	err = queue.PushOntoQueue(rc, "msgs", "chan1", rate, `[{"id":31}]`, queue.HighPriority)
	assert.NoError(t, err)

	// make sure pause bulk key do not prevent use to get from the high priority queue
	rc.Do("SET", "rate_limit_bulk:chan1", "engaged")
	rc.Do("EXPIRE", "rate_limit_bulk:chan1", 5)

	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":31}`, value)

	// make sure paused is not present for more tests
	rc.Do("DEL", "rate_limit_bulk:chan1")

	// should get next five bulk msgs fine
	for i := 10; i < 15; i++ {
		q, value, err := queue.PopFromQueue(rc, "msgs")
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)

	}

	// push a multi-message batch for a single contact
	err = queue.PushOntoQueue(rc, "msgs", "chan1", rate, `[{"id":32}, {"id":33}]`, queue.HighPriority)
	assert.NoError(t, err)

	q, value, err = queue.PopFromQueue(rc, "msgs")
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
		q, value, err := queue.PopFromQueue(rc, "msgs")
		assert.NoError(t, err)
		assert.NotEqual(t, q, queue.EmptyQueue)
		assert.Equal(t, fmt.Sprintf(`{"id":%d}`, i), value)
	}

	// next should be 30
	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.NotEqual(t, q, queue.EmptyQueue)
	assert.Equal(t, `{"id":30}`, value)

	// popping again should give us nothing since it is too soon to send 33
	q = queue.Retry
	for q == queue.Retry {
		q, value, err = queue.PopFromQueue(rc, "msgs")
	}
	assert.NoError(t, err)
	assert.Equal(t, queue.EmptyQueue, q)
	assert.Empty(t, value)

	// but if we sleep 6 seconds should get it
	time.Sleep(time.Second * 6)

	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.Equal(t, queue.WorkerToken("msgs:chan1|10"), q)
	assert.Equal(t, `{"id":33}`, value)

	// nothing should be left
	q = queue.Retry
	for q == queue.Retry {
		q, value, err = queue.PopFromQueue(rc, "msgs")
	}
	assert.NoError(t, err)
	assert.Equal(t, queue.EmptyQueue, q)
	assert.Empty(t, value)

	err = queue.PushOntoQueue(rc, "msgs", "chan1", rate, `[{"id":34}]`, queue.HighPriority)
	assert.NoError(t, err)

	rc.Do("SET", "rate_limit:chan1", "engaged")
	rc.Do("EXPIRE", "rate_limit:chan1", 5)

	// we have the rate limit set
	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	if value != "" && q != queue.EmptyQueue {
		t.Fatal("Should be throttled")
	}

	time.Sleep(2 * time.Second)

	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.Equal(t, "", value)
	assert.Equal(t, queue.Retry, q)

	assertvk.ZGetAll(t, rc, "msgs:active", map[string]float64{})
	assertvk.ZGetAll(t, rc, "msgs:throttled", map[string]float64{"msgs:chan1|10": 0})
	assertvk.ZGetAll(t, rc, "msgs:future", map[string]float64{})

	// but if we wait for the rate limit to expire
	time.Sleep(3 * time.Second)

	// next should be 34
	q, value, err = queue.PopFromQueue(rc, "msgs")
	assert.NoError(t, err)
	assert.NotEqual(t, q, queue.EmptyQueue)
	assert.Equal(t, `{"id":34}`, value)

	// nothing should be left
	q = queue.Retry
	for q == queue.Retry {
		q, value, err = queue.PopFromQueue(rc, "msgs")
	}
	assert.NoError(t, err)
	assert.Equal(t, queue.EmptyQueue, q)
	assert.Empty(t, value)
}

func TestThrottle(t *testing.T) {
	assert := assert.New(t)
	pool := getPool()
	conn := pool.Get()
	defer conn.Close()

	// start our dethrottler
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	queue.StartDethrottler(pool, quitter, wg, "msgs")

	insertCount := 30
	rate := 10

	// insert items with our set limit
	for i := range insertCount {
		err := queue.PushOntoQueue(conn, "msgs", "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.HighPriority)
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
		task, value, err = queue.PopFromQueue(conn, "msgs")
		assert.NoError(err)

		// if this wasn't throttled
		if value != "" {
			expected := fmt.Sprintf(`{"id":%d}`, curr)
			assert.Equal(expected, value, "Out of order msg")
			curr++

			err = queue.MarkComplete(conn, "msgs", task)
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
