package queue

import (
	_ "embed"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

// Priority represents the priority of an item in a queue
type Priority int64

// WorkerToken represents a token that a worker should return when a task is complete
type WorkerToken string

const (
	// HighPriority is typically used for replies to ensure they sent as soon as possible.
	HighPriority = 1

	// LowPriority is typically used for bulk messages (sent in batches). These will only be
	// processed after all high priority messages are dealt with.
	LowPriority = 0
)

const (
	// EmptyQueue means there are no items to retrive, caller should sleep and try again later
	EmptyQueue = WorkerToken("empty")

	// Retry means the caller should immediately call again to get the next value
	Retry = WorkerToken("retry")
)

//go:embed lua/push.lua
var luaPush string
var scriptPush = redis.NewScript(6, luaPush)

// PushOntoQueue pushes the passed in value to the passed in queue, making sure that no more than the
// specified transactions per second are popped off at a time. A tps value of 0 means there is no
// limit to the rate that messages can be consumed
func PushOntoQueue(conn redis.Conn, qType string, queue string, tps int, value string, priority Priority) error {
	epochMS := strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
	_, err := redis.Int(scriptPush.Do(conn, epochMS, qType, queue, tps, priority, value))
	return err
}

//go:embed lua/pop.lua
var luaPop string
var scriptPop = redis.NewScript(2, luaPop)

// PopFromQueue pops the next available message from the passed in queue. If QueueRetry
// is returned the caller should immediately make another call to get the next value. A
// worker token of EmptyQueue will be returned if there are no more items to retrive.
// Otherwise the WorkerToken should be saved in order to mark the task as complete later.
func PopFromQueue(conn redis.Conn, qType string) (WorkerToken, string, error) {
	epochMS := strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
	values, err := redis.Strings(scriptPop.Do(conn, epochMS, qType))
	if err != nil {
		slog.Error("error popping from queue", "error", err)
		return "", "", err
	}
	return WorkerToken(values[0]), values[1], nil
}

//go:embed lua/complete.lua
var luaComplete string
var scriptComplete = redis.NewScript(2, luaComplete)

// MarkComplete marks a task as complete for the passed in queue and queue result. It is
// important for callers to call this so that workers are evenly spread across all
// queues with jobs in them
func MarkComplete(conn redis.Conn, qType string, token WorkerToken) error {
	_, err := scriptComplete.Do(conn, qType, token)
	if err != nil {
		slog.Error("error marking job complete from queue", "error", err, "token", token)
	}
	return err
}

//go:embed lua/dethrottle.lua
var luaDethrottle string
var scriptDethrottle = redis.NewScript(1, luaDethrottle)

// StartDethrottler starts a goroutine responsible for dethrottling any queues that were
// throttled every second. The passed in quitter chan can be used to shut down the goroutine
func StartDethrottler(redis *redis.Pool, quitter chan bool, wg *sync.WaitGroup, qType string) {
	wg.Add(1)

	go func() {
		// figure out our next delay, we want to land just on the other side of a second boundary
		delay := time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))

		for {
			select {
			case <-quitter:
				wg.Done()
				return

			case <-time.After(delay):
				rc := redis.Get()
				_, err := scriptDethrottle.Do(rc, qType)
				if err != nil {
					slog.Error("error dethrottling", "error", err)
				}
				rc.Close()

				delay = time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))
			}
		}
	}()
}
