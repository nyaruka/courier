-- KEYS: [Queue, Channel, Ready, Waiting]
-- ARGV: [ChannelUUID, TPS, Value]
local queueKey, channelKey, readyKey, waitingKey = KEYS[1], KEYS[2], KEYS[3], KEYS[4]
local channelUUID, tps, value = ARGV[1], tonumber(ARGV[2]), ARGV[3]

local time = redis.call("TIME")
local nowSec = tonumber(time[1])
local now = nowSec + tonumber(time[2]) / 1000000

redis.call("ZADD", queueKey, now, value)

-- the latest push sets the TPS for everything queued for this channel
redis.call("HSET", channelKey, "tps", tps)
redis.call("PERSIST", channelKey)

-- a channel that's used up this second's TPS waits until the next, otherwise make it ready so that this item doesn't
-- wait on whatever the channel was waiting for, e.g. the rest of a batch
local state = redis.call("HMGET", channelKey, "sec", "used", "inflight")
if tps > 0 and tonumber(state[1]) == nowSec and (tonumber(state[2]) or 0) >= tps then
    if not redis.call("ZSCORE", readyKey, channelUUID) then
        redis.call("ZADD", waitingKey, "LT", nowSec + 1, channelUUID)
    end
    return 0
end

redis.call("ZREM", waitingKey, channelUUID)
redis.call("ZADD", readyKey, tonumber(state[3]) or 0, channelUUID)
return 1
