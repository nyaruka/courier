package queue

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/garyburd/redigo/redis"
)

// Priority represents the priority of an item in a queue
type Priority int64

// WorkerToken represents a token that a worker should return when a task is complete
type WorkerToken string

const (
	// LowPriority is our lowest priority, penalty of one day
	LowPriority = Priority(time.Hour / 1000 * 24)

	// DefaultPriority just queues according to the current time
	DefaultPriority = Priority(0)

	// HighPriority subtracts one day
	HighPriority = Priority(time.Hour / 1000 * -24)

	// HigherPriority subtracts two days
	HigherPriority = Priority(time.Hour / 1000 * -48)

	// EmptyQueue means there are no items to retrive, caller should sleep and try again later
	EmptyQueue = WorkerToken("empty")

	// Retry means the caller should immediately call again to get the next value
	Retry = WorkerToken("retry")
)

var luaPush = redis.NewScript(6, `-- KEYS: [Epoch, QueueType, QueueName, TPS, Score, Value]
	-- first push onto our specific queue
	local queueKey = KEYS[2] .. ":" .. KEYS[3] .. "|" .. KEYS[4]
	redis.call("zadd", queueKey, KEYS[5], KEYS[6])

	local tps = tonumber(KEYS[4])

	-- if we have a TPS, check whether we are currently throttled
	local curr = -1
	if tps > 0 then
   	    local tpsKey = queueKey .. ":tps:" .. KEYS[1]
	    curr = tonumber(redis.call("get", tpsKey))
	end

	-- if we aren't then add to our active
	if not curr or curr < tps then
	  redis.call("zincrby", KEYS[2] .. ":active", 0, queueKey)
	  return 1
	else 
	  return 0
    end
`)

// PushOntoQueue pushes the passed in value to the passed in queue, making sure that no more than the
// specified transactions per second are popped off at a time. A tps value of 0 means there is no
// limit to the rate that messages can be consumed
func PushOntoQueue(conn redis.Conn, qType string, queue string, tps int, value string, priority Priority) error {
	epoch := time.Now().Unix()
	score := time.Now().UnixNano()/1000 + int64(priority)
	_, err := redis.Int(luaPush.Do(conn, epoch, qType, queue, tps, strconv.FormatInt(score, 10), value))
	return err
}

var luaPop = redis.NewScript(2, `-- KEYS: [Epoch QueueType]
	-- get the first key off our active list
	local result = redis.call("zrange", KEYS[2] .. ":active", 0, 0, "WITHSCORES")
	local queue = result[1]
	local workers = result[2]

	-- nothing? return nothing
	if not queue then
		return {"empty", ""}
	end

	-- figure out our max transaction per second
	local delim = string.find(queue, "|")
	local tps = 0
	if delim then
	    tps = tonumber(string.sub(queue, delim+1))
	end

	-- if we have a tps, then check whether we exceed it
	if tps > 0 then
	    local tpsKey = queue .. ":tps:" .. KEYS[1]
	    local curr = tonumber(redis.call("get", tpsKey))
	    
		-- we are under our max tps, increase our # of transactions on this second
		if not curr or curr < tps then
	        redis.call("incr", tpsKey)
		    redis.call("expire", tpsKey, 10)
	    
		-- we are above our tps, move to our throttled queue
		else
			redis.call("zincrby", KEYS[2] .. ":throttled", workers, queue)
			redis.call("zrem", KEYS[2] .. ":active", queue)
			return {"retry", ""}
  	    end
	end

	-- pop our next value out
	result = redis.call("zrange", queue, 0, 0)

	-- if we found one
	if result[1] then
		-- then remove it from the queue
		redis.call('zremrangebyrank', queue, 0, 0)

		-- and add a worker to this queue
		redis.call("zincrby", KEYS[2] .. ":active", 1, queue)
		return {queue, result[1]}
	
	-- otherwise, the queue is empty, remove it from active
	else
		redis.call("zrem", KEYS[2] .. ":active", queue)
		return {"retry", ""}
	end
`)

// PopFromQueue pops the next available message from the passed in queue. If QueueRetry
// is returned the caller should immediately make another call to get the next value. A
// worker token of EmptyQueue will be returned if there are no more items to retrive.
// Otherwise the WorkerToken should be saved in order to mark the task as complete later.
func PopFromQueue(conn redis.Conn, qType string) (WorkerToken, string, error) {
	epoch := time.Now().Unix()
	values, err := redis.Strings(luaPop.Do(conn, strconv.FormatInt(epoch, 10), qType))
	if err != nil {
		return "", "", err
	}
	return WorkerToken(values[0]), values[1], nil
}

var luaComplete = redis.NewScript(2, `-- KEYS: [QueueType, Queue]
	-- decrement throttled if present
	local throttled = tonumber(redis.call("zadd", KEYS[1] .. ":throttled", "XX", "CH", "INCR", -1, KEYS[2]))

	-- if we didn't decrement anything, do so to our active set
	if not throttled or throttled == 0 then
		local active = tonumber(redis.call("zincrby", KEYS[1] .. ":active", -1, KEYS[2]))
		
		-- reset to zero if we somehow go below
		if active < 0 then
			redis.call("zadd", KEYS[1] .. ":active", 0, KEYS[2])
		end
	end
`)

// MarkComplete marks a task as complete for the passed in queue and queue result. It is
// important for callers to call this so that workers are evenly spread across all
// queues with jobs in them
func MarkComplete(conn redis.Conn, qType string, token WorkerToken) error {
	_, err := luaComplete.Do(conn, qType, token)
	return err
}

var luaDethrottle = redis.NewScript(1, `-- KEYS: [QueueType]
	-- get all the keys from our throttle list
	local throttled = redis.call("zrange", KEYS[1] .. ":throttled", 0, -1, "WITHSCORES")

	-- add them to our active list
	if next(throttled) then
		local activeKey = KEYS[1] .. ":active"
		for i=1,#throttled,2 do
			redis.call("zincrby", activeKey, throttled[i+1], throttled[i])
		end
		redis.call("del", KEYS[1] .. ":throttled")
	end
`)

// StartDethrottler starts a goroutine responsible for dethrottling any queues that were
// throttled every second. The passed in quitter chan can be used to shut down the goroutine
func StartDethrottler(redis *redis.Pool, quitter chan bool, wg *sync.WaitGroup, qType string) {
	go func() {
		wg.Add(1)

		// figure out our next delay, we want to land just on the other side of a second boundary
		delay := time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))

		for true {
			select {
			case <-quitter:
				wg.Done()
				return

			case <-time.After(delay):
				conn := redis.Get()
				_, err := luaDethrottle.Do(conn, qType)
				if err != nil {
					fmt.Println(err)
				}
				conn.Close()

				delay = time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))
			}
		}
	}()
}
