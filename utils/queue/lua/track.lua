-- KEYS: [Unacked, AckLimits]
-- ARGV: [MsgUUID, MaxUnacked, MaxAckLag, UnackedTTL]
local unackedKey, limitsKey = KEYS[1], KEYS[2]
local msgUUID, maxUnacked, maxAckLag, unackedTTL = ARGV[1], ARGV[2], ARGV[3], tonumber(ARGV[4])

local time = redis.call("TIME")
local now = tonumber(time[1]) + tonumber(time[2]) / 1000000

redis.call("ZADD", unackedKey, now, msgUUID)
redis.call("HSET", limitsKey, "max_unacked", maxUnacked, "max_ack_lag", maxAckLag)

-- both expire once the channel stops sending, by when anything still unacknowledged would be forgotten anyway
redis.call("EXPIRE", unackedKey, unackedTTL)
redis.call("EXPIRE", limitsKey, unackedTTL)
