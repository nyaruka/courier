-- KEYS: [Channel, Ready, Waiting]
-- ARGV: [ChannelUUID]
local channelKey, readyKey, waitingKey = KEYS[1], KEYS[2], KEYS[3]
local channelUUID = ARGV[1]

local inflight = redis.call("HINCRBY", channelKey, "inflight", -1)
if inflight < 0 then
    inflight = 0
    redis.call("HSET", channelKey, "inflight", 0)
end

if redis.call("ZSCORE", readyKey, channelUUID) then
    redis.call("ZADD", readyKey, inflight, channelUUID)

-- a channel that's neither ready nor waiting has nothing queued, so once nothing is in flight let its state expire
elseif inflight == 0 and not redis.call("ZSCORE", waitingKey, channelUUID) then
    redis.call("EXPIRE", channelKey, 60)
end
