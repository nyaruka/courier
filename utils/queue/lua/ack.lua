-- KEYS: [Unacked, Acks, AckLimits]
-- ARGV: [MsgUUID, MaxUnacked, MaxAckLag, AckRateWindow, UnackedRetention]
local unackedKey, acksKey, limitsKey = KEYS[1], KEYS[2], KEYS[3]
local msgUUID, maxUnacked, maxAckLag = ARGV[1], tonumber(ARGV[2]), tonumber(ARGV[3])
local window, retention = tonumber(ARGV[4]), tonumber(ARGV[5])

-- the channel's current limits are applied here as well as on each send, as a channel waiting on its limits isn't
-- sending, and otherwise raising or removing them wouldn't reach it
if maxUnacked > 0 or maxAckLag > 0 then
    redis.call("HSET", limitsKey, "max_unacked", maxUnacked, "max_ack_lag", maxAckLag)
    redis.call("EXPIRE", limitsKey, retention)
else
    redis.call("DEL", limitsKey)
end

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

redis.call("EXPIRE", acksKey, retention)
return 1
