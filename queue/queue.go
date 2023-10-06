package queue

import (
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

var luaPush = redis.NewScript(6, `-- KEYS: [EpochMS, QueueType, QueueName, TPS, Priority, Value]
	-- first push onto our specific queue
	-- our queue name is built from the type, name and tps, usually something like: "msgs:uuid1-uuid2-uuid3-uuid4|tps"
	local queueKey = KEYS[2] .. ":" .. KEYS[3] .. "|" .. KEYS[4]

	-- our priority queue name also includes the priority of the message (we have one queue for default and one for bulk)
	local priorityQueueKey = queueKey .. "/" .. KEYS[5]
	redis.call("zadd", priorityQueueKey, KEYS[1], KEYS[6])

	local tps = tonumber(KEYS[4])

	-- if we have a TPS, check whether we are currently throttled
	local curr = -1
	if tps > 0 then
   	    local tpsKey = queueKey .. ":tps:" .. math.floor(KEYS[1])
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
	epochMS := strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
	_, err := redis.Int(luaPush.Do(conn, epochMS, qType, queue, tps, priority, value))
	return err
}

var luaPop = redis.NewScript(2, `-- KEYS: [EpochMS QueueType]
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
	local tpsKey = ""

	local queueName = ""

	if delim then
	    queueName = string.sub(queue, string.len(KEYS[2])+2, delim-1)
	    tps = tonumber(string.sub(queue, delim+1))
	end

	if queueName then
		local rateLimitKey = "rate_limit:" .. queueName
		local rateLimitEngaged = redis.call("get", rateLimitKey)
		if rateLimitEngaged then
			redis.call("zincrby", KEYS[2] .. ":throttled", workers, queue)
			redis.call("zrem", KEYS[2] .. ":active", queue)
			return {"retry", ""}
		end
	end

	-- if we have a tps, then check whether we exceed it
	if tps > 0 then
	    tpsKey = queue .. ":tps:" .. math.floor(KEYS[1])
	    local curr = redis.call("get", tpsKey)
	    
		-- we are at or above our tps, move to our throttled queue
		if curr and tonumber(curr) >= tps then 
			redis.call("zincrby", KEYS[2] .. ":throttled", workers, queue)
			redis.call("zrem", KEYS[2] .. ":active", queue)
			return {"retry", ""}
  	    end
	end

	-- pop our next value out, first from our default queue
	local resultQueue = queue .. "/1"
	local result = redis.call("zrangebyscore", resultQueue, 0, "+inf", "WITHSCORES", "LIMIT", 0, 1)
	
	-- keep track as to whether this result is in the future (and therefore ineligible)
	local isFutureResult = result[1] and tonumber(result[2]) > tonumber(KEYS[1])

	-- if we didn't find one, try again from our bulk queue
	if not result[1] or isFutureResult then
		-- check if we are rate limited for bulk queue
		local rateLimitBulkKey = "rate_limit_bulk:" .. queueName
		local rateLimitBulk = redis.call("get", rateLimitBulkKey)
		if rateLimitBulk then
			return {"retry", ""}
		end

		-- we are not pause check our bulk queue
		local bulkQueue = queue .. "/0"
		local bulkResult = redis.call("zrangebyscore", bulkQueue, 0, "+inf", "WITHSCORES", "LIMIT", 0, 1)

		-- if we got a result
		if bulkResult[1] then
			-- if it is in the future, set ourselves as in the future
			if tonumber(bulkResult[2]) > tonumber(KEYS[1]) then
				isFutureResult = true
			
			-- otherwise, this is a valid result
			else 
				redis.call("echo", "found result")
				isFutureResult = false
				result = bulkResult
				resultQueue = bulkQueue
			end
		end
	end

	-- if we found one
	if result[1] and not isFutureResult then
		-- then remove it from the queue
		redis.call('zremrangebyrank', resultQueue, 0, 0)

		-- and add a worker to this queue
		redis.call("zincrby", KEYS[2] .. ":active", 1, queue)

		-- parse it as JSON to get the first element out
		local valueList = cjson.decode(result[1])
		local popValue = cjson.encode(valueList[1])
		table.remove(valueList, 1)

		-- increment our tps for this second if we have a limit
		if tps > 0 then 
		    redis.call("incrby", tpsKey, popValue["tps_cost"] or 1)
		    redis.call("expire", tpsKey, 10)
		end 

		-- encode it back if there is anything left
		if table.getn(valueList) > 0 then
		    local remaining = cjson.encode(valueList)
	        
            -- schedule it in the future 3 seconds on our main queue
            redis.call("zadd", queue .. "/1", tonumber(KEYS[1]) + 3, remaining)
            redis.call("zincrby", KEYS[2] .. ":future", 0, queue)
		end

		return {queue, popValue}

	-- otherwise, the queue only contains future results, remove from active and add to future, have the caller retry
	elseif isFutureResult then
	    redis.call("zincrby", KEYS[2] .. ":future", 0, queue)
	    redis.call("zrem", KEYS[2] .. ":active", queue)
		return {"retry", ""}
	
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
	epochMS := strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
	values, err := redis.Strings(luaPop.Do(conn, epochMS, qType))
	if err != nil {
		slog.Error("error popping from queue", "error", err)
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

	-- get all the keys in the future
	local future = redis.call("zrange", KEYS[1] .. ":future", 0, -1, "WITHSCORES")

	-- add them to our active list
	if next(future) then
		local activeKey = KEYS[1] .. ":active"
		for i=1,#future,2 do
			redis.call("zincrby", activeKey, future[i+1], future[i])
		end
		redis.call("del", KEYS[1] .. ":future")
	end
`)

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
				conn := redis.Get()
				_, err := luaDethrottle.Do(conn, qType)
				if err != nil {
					slog.Error("error dethrottling", "error", err)
				}
				conn.Close()

				delay = time.Second - time.Duration(time.Now().UnixNano()%int64(time.Second))
			}
		}
	}()
}
