-- KEYS: [QueueType]

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