package celery

import (
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
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
func TestQueue(t *testing.T) {
	pool := getPool()
	defer pool.Close()

	conn := pool.Get()
	defer conn.Close()

	// queue to our handler queue
	conn.Send("multi")
	err := QueueEmptyTask(conn, "handler", "handle_event_task")
	if err != nil {
		t.Error(err)
	}
	_, err = conn.Do("EXEC")
	if err != nil {
		t.Error(err)
	}

	// check whether things look right
	taskJSON, err := redis.String(conn.Do("LPOP", "handler"))
	if err != nil {
		t.Errorf("should have value in handler queue: %s", err)
	}

	// make sure our task is valid json
	task := Task{}
	err = json.Unmarshal([]byte(taskJSON), &task)
	if err != nil {
		t.Errorf("should be JSON: %s", err)
	}

	// and is against the right queue
	if task.Properties.DeliveryInfo.RoutingKey != "handler" {
		t.Errorf("task should have handler as routing key")
	}
}
