package queue

import (
	_ "embed"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// all keys share this hash tag so that scripts can use them together in cluster mode. Each channel has a queue
// of high and bulk priority items ({msgs}:q:<uuid>/1 and /0) and a hash of its state ({msgs}:ch:<uuid>), and is
// either ready to be popped from (in {msgs}:ready, scored by sends in flight) or waiting until a time (in
// {msgs}:waiting, scored by that time).
const keyBase = "{msgs}"

const (
	readyKey   = keyBase + ":ready"
	waitingKey = keyBase + ":waiting"
)

// the most waiting channels that a single pop will make ready
const wakeLimit = 100

func queueKey(channelUUID string, priority Priority) string {
	return keyBase + ":q:" + channelUUID + "/" + strconv.Itoa(int(priority))
}

func channelKey(channelUUID string) string {
	return keyBase + ":ch:" + channelUUID
}

//go:embed lua/push.lua
var luaPush string
var scriptPush = redis.NewScript(4, luaPush)

// PushOntoQueue pushes the passed in value to the queue for the given channel, making sure that no more than the
// specified transactions per second are popped off at a time. A tps value of 0 means there is no limit to the rate
// that messages can be consumed. The TPS applies to everything already queued for the channel.
func PushOntoQueue(conn redis.Conn, channelUUID string, tps int, value string, priority Priority) error {
	_, err := redis.Int(scriptPush.Do(conn, queueKey(channelUUID, priority), channelKey(channelUUID), readyKey, waitingKey, channelUUID, tps, value))
	return err
}

//go:embed lua/pop.lua
var luaPop string
var scriptPop = redis.NewScript(2, luaPop)

// queues are read in both the current and legacy layouts while producers move from one to the other, alternating
// between them so that neither starves the other
var popCount atomic.Uint64

// PopFromQueue pops the next available message from the queue. If QueueRetry is returned the caller should
// immediately make another call to get the next value. A worker token of EmptyQueue will be returned if there are
// no more items to retrive. Otherwise the WorkerToken should be saved in order to mark the task as complete later.
func PopFromQueue(conn redis.Conn) (WorkerToken, string, error) {
	first, second := pop, popLegacy
	if popCount.Add(1)%2 == 0 {
		first, second = second, first
	}

	token, value, err := first(conn)
	if err != nil || token != EmptyQueue {
		return token, value, err
	}
	return second(conn)
}

func pop(conn redis.Conn) (WorkerToken, string, error) {
	values, err := redis.Strings(scriptPop.Do(conn, readyKey, waitingKey, keyBase, wakeLimit))
	if err != nil {
		slog.Error("error popping from queue", "error", err)
		return "", "", err
	}
	return WorkerToken(values[0]), values[1], nil
}

// Size returns the number of high and bulk priority items queued across all channels, in both layouts
func Size(conn redis.Conn) (high, bulk int, err error) {
	var queues []string

	for _, set := range []string{readyKey, waitingKey} {
		uuids, err := redis.Strings(conn.Do("ZRANGE", set, 0, -1))
		if err != nil {
			return 0, 0, err
		}
		for _, uuid := range uuids {
			queues = append(queues, keyBase+":q:"+uuid)
		}
	}

	// a legacy queue can be both active and future
	legacy := make(map[string]bool)
	for _, set := range []string{":active", ":throttled", ":future"} {
		names, err := redis.Strings(conn.Do("ZRANGE", legacyQueueType+set, 0, -1))
		if err != nil {
			return 0, 0, err
		}
		for _, name := range names {
			if !legacy[name] {
				legacy[name] = true
				queues = append(queues, name)
			}
		}
	}

	for _, q := range queues {
		h, err := redis.Int(conn.Do("ZCARD", q+"/1"))
		if err != nil {
			return 0, 0, err
		}
		b, err := redis.Int(conn.Do("ZCARD", q+"/0"))
		if err != nil {
			return 0, 0, err
		}
		high += h
		bulk += b
	}
	return high, bulk, nil
}

//go:embed lua/complete.lua
var luaComplete string
var scriptComplete = redis.NewScript(3, luaComplete)

// MarkComplete marks a task as complete for the passed in queue and queue result. It is
// important for callers to call this so that workers are evenly spread across all
// queues with jobs in them
func MarkComplete(conn redis.Conn, token WorkerToken) error {
	var err error
	if isLegacyToken(token) {
		_, err = scriptLegacyComplete.Do(conn, legacyQueueType, token)
	} else {
		channelUUID := string(token)
		_, err = scriptComplete.Do(conn, channelKey(channelUUID), readyKey, waitingKey, channelUUID)
	}
	if err != nil {
		slog.Error("error marking job complete from queue", "error", err, "token", token)
	}
	return err
}

// the legacy layout has queues named by channel and TPS, i.e. msgs:<uuid>|<tps>, which are also its worker tokens
const legacyQueueType = "msgs"

func isLegacyToken(token WorkerToken) bool {
	return strings.Contains(string(token), "|")
}

//go:embed lua/legacy_push.lua
var luaLegacyPush string
var scriptLegacyPush = redis.NewScript(6, luaLegacyPush)

func pushOntoLegacyQueue(conn redis.Conn, channelUUID string, tps int, value string, priority Priority) error {
	_, err := redis.Int(scriptLegacyPush.Do(conn, epochMS(), legacyQueueType, channelUUID, tps, priority, value))
	return err
}

//go:embed lua/legacy_pop.lua
var luaLegacyPop string
var scriptLegacyPop = redis.NewScript(2, luaLegacyPop)

func popLegacy(conn redis.Conn) (WorkerToken, string, error) {
	values, err := redis.Strings(scriptLegacyPop.Do(conn, epochMS(), legacyQueueType, keyBase))
	if err != nil {
		slog.Error("error popping from legacy queue", "error", err)
		return "", "", err
	}
	return WorkerToken(values[0]), values[1], nil
}

//go:embed lua/legacy_complete.lua
var luaLegacyComplete string
var scriptLegacyComplete = redis.NewScript(2, luaLegacyComplete)

//go:embed lua/legacy_dethrottle.lua
var luaLegacyDethrottle string
var scriptLegacyDethrottle = redis.NewScript(1, luaLegacyDethrottle)

// StartDethrottler starts a goroutine responsible for dethrottling any queues in the legacy layout that were
// throttled every second. The passed in quitter chan can be used to shut down the goroutine
func StartDethrottler(redis *redis.Pool, quitter chan bool, wg *sync.WaitGroup) {
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
				_, err := scriptLegacyDethrottle.Do(rc, legacyQueueType)
				if err != nil {
					slog.Error("error dethrottling", "error", err)
				}
				rc.Close()

				delay = time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))
			}
		}
	}()
}

func epochMS() string {
	return strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
}
