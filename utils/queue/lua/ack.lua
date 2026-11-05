-- KEYS: [Unacked, Acks]
-- ARGV: [MsgUUID, AckRateWindow, UnackedTTL]
local unackedKey, acksKey = KEYS[1], KEYS[2]
local msgUUID, window, unackedTTL = ARGV[1], tonumber(ARGV[2]), tonumber(ARGV[3])

-- only a send we were still waiting on counts towards the acknowledgement rate, so repeated reports don't inflate it
if redis.call("ZREM", unackedKey, msgUUID) == 0 then
    return 0
end

-- acknowledgements are counted per second in a ring of slots, one more than the window so that the slot of the
-- current second, which is still filling, never overwrites one that's in the window
local nowSec = tonumber(redis.call("TIME")[1])
local slot = nowSec % (window + 1)

if tonumber(redis.call("HGET", acksKey, "s" .. slot)) == nowSec then
    redis.call("HINCRBY", acksKey, "c" .. slot, 1)
else
    redis.call("HSET", acksKey, "s" .. slot, nowSec, "c" .. slot, 1)
end

redis.call("EXPIRE", acksKey, unackedTTL)
return 1
