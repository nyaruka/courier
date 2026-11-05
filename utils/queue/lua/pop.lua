-- KEYS: [Ready, Waiting]
-- ARGV: [KeyBase, WakeLimit, UnackedTTL, AckLagFloor, AckRateWindow, UnackedRetention]
local readyKey, waitingKey = KEYS[1], KEYS[2]
local keyBase, wakeLimit = ARGV[1], tonumber(ARGV[2])
local unackedTTL, ackLagFloor, ackRateWindow = tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[5])
local unackedRetention = tonumber(ARGV[6])

local time = redis.call("TIME")
local nowSec = tonumber(time[1])
local now = nowSec + tonumber(time[2]) / 1000000

-- channel keys share our hash tag so are safe to construct here even in cluster mode
local function channelKey(uuid)
    return keyBase .. ":ch:" .. uuid
end

-- make ready any channels whose wait is over
for _, uuid in ipairs(redis.call("ZRANGEBYSCORE", waitingKey, "-inf", now, "LIMIT", 0, wakeLimit)) do
    redis.call("ZREM", waitingKey, uuid)
    redis.call("ZADD", readyKey, tonumber(redis.call("HGET", channelKey(uuid), "inflight")) or 0, uuid)
end

-- take the ready channel with the fewest sends in flight
local uuid = redis.call("ZRANGE", readyKey, 0, 0)[1]
if not uuid then
    return {"empty", ""}
end

local chKey = channelKey(uuid)

local function park(untilTime)
    redis.call("ZREM", readyKey, uuid)
    redis.call("ZADD", waitingKey, untilTime, uuid)
    return {"retry", ""}
end

local state = redis.call("HMGET", chKey, "tps", "sec", "used", "inflight")
local tps = tonumber(state[1]) or 0
local used = 0
if tonumber(state[2]) == nowSec then
    used = tonumber(state[3]) or 0
end

if tps > 0 and used >= tps then
    return park(nowSec + 1)
end

local function head(key)
    local result = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
    if result[1] then
        return result[1], tonumber(result[2])
    end
    return nil, nil
end

-- high priority items first, then bulk
local highKey, bulkKey = keyBase .. ":q:" .. uuid .. "/1", keyBase .. ":q:" .. uuid .. "/0"
local highValue, highScore = head(highKey)
local bulkValue, bulkScore = head(bulkKey)
local fromKey, value

if highValue and highScore <= now then
    fromKey, value = highKey, highValue
elseif bulkValue and bulkScore <= now then
    fromKey, value = bulkKey, bulkValue
elseif highValue or bulkValue then
    -- only items scheduled for later, so wait until the first of those
    return park(math.min(highScore or math.huge, bulkScore or math.huge))
else
    redis.call("ZREM", readyKey, uuid)

    -- nothing queued so let the channel's state expire, but not before this second's usage is done. Sends in flight
    -- are forgotten so that any never marked complete, e.g. by a worker that died, don't count against the channel
    -- forever - those that are still in flight just complete to zero.
    redis.call("HSET", chKey, "inflight", 0)
    redis.call("EXPIRE", chKey, 60)
    return {"retry", ""}
end

-- a channel with unacknowledged send limits waits while the provider has too many of its sends that it hasn't
-- acknowledged, or while those would take too long to clear at the rate it's been acknowledging them. Sends never
-- acknowledged are forgotten after a while, so lost acknowledgements hold back a channel at its count limit for at most
-- that long.
local limits = redis.call("HMGET", keyBase .. ":acklimits:" .. uuid, "max_unacked", "max_ack_lag")
local maxUnacked, maxAckLag = tonumber(limits[1]) or 0, tonumber(limits[2]) or 0

if maxUnacked > 0 or maxAckLag > 0 then
    local unackedKey = keyBase .. ":unacked:" .. uuid
    -- sends are kept for longer than they count, so that a late acknowledgement still counts towards the rate
    redis.call("ZREMRANGEBYSCORE", unackedKey, "-inf", now - unackedRetention)
    local unacked = redis.call("ZCOUNT", unackedKey, now - unackedTTL, "+inf")

    -- sends in flight aren't tracked until they complete, so they count too or concurrent workers could overshoot
    if maxUnacked > 0 and unacked + (tonumber(state[4]) or 0) >= maxUnacked then
        return park(nowSec + 1)
    end

    -- the lag is the time the backlog would take to clear at the rate acknowledgements arrived over the last few whole
    -- seconds - which unlike the age of the oldest send, rises as soon as the backlog does. Only sends from the last two
    -- lags count, so that acknowledgements lost in bulk, e.g. when Kannel restarts, stop holding the channel back once
    -- they're that old, and a stalled SMSC gets a few sends through to test it every two lags. It also only applies once
    -- there's a real backlog, so that a few lost acknowledgements can't park a quiet channel.
    local recent = 0
    if maxAckLag > 0 then
        recent = redis.call("ZCOUNT", unackedKey, now - math.min(2 * maxAckLag, unackedTTL), "+inf")
    end

    if recent >= ackLagFloor then
        local acks = redis.call("HGETALL", keyBase .. ":acks:" .. uuid)
        local slots = {}
        for i = 1, #acks, 2 do
            slots[acks[i]] = tonumber(acks[i + 1])
        end

        local acked = 0
        for s = 0, ackRateWindow do
            local sec = slots["s" .. s]
            if sec and sec < nowSec and sec >= nowSec - ackRateWindow then
                acked = acked + (slots["c" .. s] or 0)
            end
        end

        -- the rate is over the whole window, so it's underestimated for a few seconds after a quiet spell and a channel
        -- starting a burst may wait briefly until acknowledgements catch up
        if acked == 0 or recent / (acked / ackRateWindow) > maxAckLag then
            return park(nowSec + 1)
        end
    end
end

redis.call("ZREM", fromKey, value)
redis.call("ZADD", readyKey, redis.call("HINCRBY", chKey, "inflight", 1), uuid)

-- items are batches of messages for a single contact, of which we take the first
local batch = cjson.decode(value)
local popped = table.remove(batch, 1)

if tps > 0 then
    local cost = 1
    if type(popped) == "table" and type(popped["tps_cost"]) == "number" then
        cost = math.max(1, math.floor(popped["tps_cost"]))
    end
    redis.call("HSET", chKey, "sec", nowSec, "used", used + cost)
end

-- and schedule the rest a few seconds out so they're sent in order
if #batch > 0 then
    redis.call("ZADD", highKey, now + 3, cjson.encode(batch))
end

return {uuid, cjson.encode(popped)}
