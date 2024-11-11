-- KEYS: [EpochMS QueueType]

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
