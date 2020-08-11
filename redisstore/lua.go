package redisstore

const luaTemplate = `
local C_EXPIRE  = 'EXPIRE'
local C_HGETALL = 'HGETALL'
local C_HSET    = 'HSET'
local F_START   = 's'
local F_TICK    = 't'
local F_TOKENS  = 'k'

-- speed up access to next
local next = next

local key       = KEYS[1]
local now       = tonumber(ARGV[1]) -- current unix time in nanoseconds
local maxtokens = %d
local interval  = %d
local rate      = %f
local ttl       = %d

-- hgetall gets all the fields as a lua table.
local hgetall = function (key)
  local data = redis.call(C_HGETALL, key)
  local result = {}
  for i = 1, #data, 2 do
    result[data[i]] = data[i+1]
  end
  return result
end

-- availabletokens returns the number of available tokens given the last tick,
-- current tick, max, and fill rate.
local availabletokens = function (last, curr, max, fillrate)
  local delta = curr - last
  local available = delta * fillrate
  if available > max then
    available = max
  end
  return available
end

-- tick returns the total number of times the interval has occurred between
-- start and current.
local tick = function (start, curr, interval)
  return math.floor((curr - start) / interval)
end


--
-- begin exec
--

-- reset TTL, we saw the key
redis.call(C_EXPIRE, key, ttl)

local data = hgetall(key)
local start, lasttick, tokens
if next(data) == nil then
  start    = now
  lasttick = 0
  tokens   = maxtokens-1
  redis.call(C_HSET, key, F_START, start, F_TICK, lasttick, F_TOKENS, tokens)
  redis.call(C_EXPIRE, key, ttl)

  local nexttime = start + interval
  return {tokens, nexttime, true}
else
  start    = tonumber(data[F_START])
  lasttick = tonumber(data[F_TICK])
  tokens   = tonumber(data[F_TOKENS])
end

local currtick = tick(start, now, interval)
local nexttime = start + ((currtick+1) * interval)

if lasttick < currtick then
  tokens = availabletokens(lasttick, currtick, maxtokens, rate)
  lasttick = currtick
  redis.call(C_HSET, key, F_TICK, lasttick, F_TOKENS, tokens)
  redis.call(C_EXPIRE, key, ttl)
end

if tokens > 0 then
  tokens = tokens-1
  redis.call(C_HSET, key, F_TOKENS, tokens)
  redis.call(C_EXPIRE, key, ttl)

  return {tokens, nexttime, true}
end

return {0, nexttime, false}
`
