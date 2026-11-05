-- KEYS: [Unacked, AckLimits]
-- ARGV: [MsgUUID, MaxUnacked, MaxAckLag, UnackedRetention, AckLimitsTTL]
local unackedKey, limitsKey = KEYS[1], KEYS[2]
local msgUUID, maxUnacked, maxAckLag = ARGV[1], ARGV[2], ARGV[3]
local retention, limitsTTL = tonumber(ARGV[4]), tonumber(ARGV[5])

local time = redis.call("TIME")
local now = tonumber(time[1]) + tonumber(time[2]) / 1000000

redis.call("ZADD", unackedKey, now, msgUUID)
redis.call("HSET", limitsKey, "max_unacked", maxUnacked, "max_ack_lag", maxAckLag)

-- the sends expire once the channel stops sending, by when anything still unacknowledged would be forgotten anyway
redis.call("EXPIRE", unackedKey, retention)

-- but the limits soon lapse unless sends or acknowledgements keep them current - see ack.lua
redis.call("EXPIRE", limitsKey, limitsTTL)
