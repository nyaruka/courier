-- KEYS: [QueueType, Queue]

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