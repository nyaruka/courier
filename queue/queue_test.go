package queue

import (
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
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
	assert := assert.New(t)

	// start our dethrottler
	pool := getPool()
	conn := pool.Get()
	defer conn.Close()
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	StartDethrottler(pool, quitter, wg, "msgs")
	defer close(quitter)

	rate := 10

	for i := 0; i < 20; i++ {
		err := PushOntoQueue(conn, "msgs", "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), LowPriority)
		assert.NoError(err)
	}

	// get ourselves aligned with a second boundary
	delay := time.Second*2 - time.Duration(time.Now().UnixNano()%int64(time.Second))
	time.Sleep(delay)

	conn.Do("SET", "rate_limit_bulk:chan1", "engaged")
	conn.Do("EXPIRE", "rate_limit_bulk:chan1", 5)

	// we have the rate limit set,
	queue, value, err := PopFromQueue(conn, "msgs")
	assert.NoError(err)
	if value != "" && queue != EmptyQueue {
		t.Fatal("Should be paused")
	}

	// When the redis paused key is remove, we get the values from bulk queue/low priority
	conn.Do("DEL", "rate_limit_bulk:chan1")

	// pop 10 items off
	for i := 0; i < 10; i++ {
		queue, value, err := PopFromQueue(conn, "msgs")
		assert.NotEqual(queue, EmptyQueue)
		assert.Equal(fmt.Sprintf(`{"id":%d}`, i), value)
		assert.NoError(err)
	}

	// next value should be throttled
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	if value != "" && queue != EmptyQueue {
		t.Fatal("Should be throttled")
	}

	// check our redis state
	count, err := redis.Int(conn.Do("ZCARD", "msgs:throttled"))
	assert.NoError(err)
	assert.Equal(1, count, "Expected chan1 to be throttled")

	count, err = redis.Int(conn.Do("ZCARD", "msgs:active"))
	assert.NoError(err)
	assert.Equal(0, count, "Expected chan1 to not be active")

	// adding more items shouldn't change that
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	if value != "" && queue != EmptyQueue {
		t.Fatal("Should be throttled")
	}
	err = PushOntoQueue(conn, "msgs", "chan1", rate, `[{"id":30}]`, LowPriority)
	assert.NoError(err)

	count, err = redis.Int(conn.Do("ZCARD", "msgs:throttled"))
	assert.NoError(err)
	assert.Equal(1, count, "Expected chan1 to be throttled")

	count, err = redis.Int(conn.Do("ZCARD", "msgs:active"))
	assert.NoError(err)
	assert.Equal(0, count, "Expected chan1 to not be active")

	// but if we wait, our next msg should be our highest priority
	time.Sleep(time.Second)
	err = PushOntoQueue(conn, "msgs", "chan1", rate, `[{"id":31}]`, HighPriority)
	assert.NoError(err)

	// make sure pause bulk key do not prevent use to get from the high priority queue
	conn.Do("SET", "rate_limit_bulk:chan1", "engaged")
	conn.Do("EXPIRE", "rate_limit_bulk:chan1", 5)

	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	assert.Equal(WorkerToken("msgs:chan1|10"), queue)
	assert.Equal(`{"id":31}`, value)

	// make sure paused is not present for more tests
	conn.Do("DEL", "rate_limit_bulk:chan1")

	// should get next five bulk msgs fine
	for i := 10; i < 15; i++ {
		queue, value, err := PopFromQueue(conn, "msgs")
		assert.NotEqual(queue, EmptyQueue)
		assert.Equal(fmt.Sprintf(`{"id":%d}`, i), value)
		assert.NoError(err)
	}

	// push on a compound message
	err = PushOntoQueue(conn, "msgs", "chan1", rate, `[{"id":32}, {"id":33}]`, HighPriority)
	assert.NoError(err)

	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	assert.Equal(WorkerToken("msgs:chan1|10"), queue)
	assert.Equal(`{"id":32}`, value)

	// sleep a few seconds
	time.Sleep(2 * time.Second)

	// pop remaining bulk off
	for i := 15; i < 20; i++ {
		queue, value, err := PopFromQueue(conn, "msgs")
		assert.NotEqual(queue, EmptyQueue)
		assert.Equal(fmt.Sprintf(`{"id":%d}`, i), value)
		assert.NoError(err)
	}

	// next should be 30
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NotEqual(queue, EmptyQueue)
	assert.Equal(`{"id":30}`, value)
	assert.NoError(err)

	// popping again should give us nothing since it is too soon to send 33
	queue = Retry
	for queue == Retry {
		queue, value, err = PopFromQueue(conn, "msgs")
	}
	assert.NoError(err)
	assert.Equal(EmptyQueue, queue)
	assert.Empty(value)

	// but if we sleep 6 seconds should get it
	time.Sleep(time.Second * 6)

	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	assert.Equal(WorkerToken("msgs:chan1|10"), queue)
	assert.Equal(`{"id":33}`, value)

	// nothing should be left
	queue = Retry
	for queue == Retry {
		queue, value, err = PopFromQueue(conn, "msgs")
	}
	assert.NoError(err)
	assert.Equal(EmptyQueue, queue)
	assert.Empty(value)

	err = PushOntoQueue(conn, "msgs", "chan1", rate, `[{"id":34}]`, HighPriority)
	assert.NoError(err)

	conn.Do("SET", "rate_limit:chan1", "engaged")
	conn.Do("EXPIRE", "rate_limit:chan1", 5)

	// we have the rate limit set,
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	if value != "" && queue != EmptyQueue {
		t.Fatal("Should be throttled")
	}

	time.Sleep(2 * time.Second)
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NoError(err)
	if value != "" && queue != EmptyQueue {
		t.Fatal("Should be throttled")
	}

	count, err = redis.Int(conn.Do("ZCARD", "msgs:throttled"))
	assert.NoError(err)
	assert.Equal(1, count, "Expected chan1 to be throttled")

	count, err = redis.Int(conn.Do("ZCARD", "msgs:active"))
	assert.NoError(err)
	assert.Equal(0, count, "Expected chan1 to not be active")

	// but if we wait for the rate limit to expire
	time.Sleep(3 * time.Second)

	// next should be 34
	queue, value, err = PopFromQueue(conn, "msgs")
	assert.NotEqual(queue, EmptyQueue)
	assert.Equal(`{"id":34}`, value)
	assert.NoError(err)

	// nothing should be left
	queue = Retry
	for queue == Retry {
		queue, value, err = PopFromQueue(conn, "msgs")
	}
	assert.NoError(err)
	assert.Equal(EmptyQueue, queue)
	assert.Empty(value)

}

func TestThrottle(t *testing.T) {
	assert := assert.New(t)
	pool := getPool()
	conn := pool.Get()
	defer conn.Close()

	// start our dethrottler
	quitter := make(chan bool)
	wg := &sync.WaitGroup{}
	StartDethrottler(pool, quitter, wg, "msgs")

	insertCount := 30
	rate := 10

	// insert items with our set limit
	for i := 0; i < insertCount; i++ {
		err := PushOntoQueue(conn, "msgs", "chan1", rate, fmt.Sprintf(`[{"id":%d}]`, i), HighPriority)
		assert.NoError(err)
		time.Sleep(1 * time.Microsecond)
	}

	// start timing
	start := time.Now()
	curr := 0
	var task WorkerToken
	var err error
	var value string
	for curr < insertCount {
		task, value, err = PopFromQueue(conn, "msgs")
		assert.NoError(err)

		// if this wasn't throttled
		if value != "" {
			expected := fmt.Sprintf(`{"id":%d}`, curr)
			assert.Equal(expected, value, "Out of order msg")
			curr++

			err = MarkComplete(conn, "msgs", task)
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

func BenchmarkQueue(b *testing.B) {
	assert := assert.New(b)
	pool := getPool()
	conn := pool.Get()
	defer conn.Close()

	for i := 0; i < b.N; i++ {
		insertValue := fmt.Sprintf(`{"id":%d}`, i)
		err := PushOntoQueue(conn, "msgs", "chan1", 0, "["+insertValue+"]", HighPriority)
		assert.NoError(err)

		queue, value, err := PopFromQueue(conn, "msgs")
		assert.NoError(err)
		assert.Equal(WorkerToken("msgs:chan1|0"), queue, "Mismatched queue")
		assert.Equal(insertValue, value, "Mismatched value")

		err = MarkComplete(conn, "msgs", queue)
		assert.NoError(err)
	}
}
