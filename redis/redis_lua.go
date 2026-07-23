// Package redis 提供 Redis Lua 脚本定义
//
// @author zwei
package redis

// luaHashSetWithExpire 原子写入 Hash 字段并按需设置过期时间
// KEYS[1] = Hash 键，ARGV[1] = 过期时间（毫秒，0 表示不过期），ARGV[2..n] = 字段名与值交替排列
const luaHashSetWithExpire = `
for i = 2, #ARGV, 2 do
    redis.call('HSET', KEYS[1], ARGV[i], ARGV[i + 1])
end
local ttlMs = tonumber(ARGV[1])
if ttlMs > 0 then
    redis.call('PEXPIRE', KEYS[1], ttlMs)
end
return 1
`

// luaRateLimitAllow 滑动窗口限流：清理过期成员后尝试消耗 n 次配额
// KEYS[1] = 限流键
// ARGV[1] = limit（窗口内最大次数）
// ARGV[2] = n（本次消耗次数）
// ARGV[3] = windowMs（窗口毫秒）
// ARGV[4] = uniquePrefix（本次请求唯一前缀，保证 ZSET member 不碰撞）
// 返回 1 表示放行，0 表示超限
const luaRateLimitAllow = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local n = tonumber(ARGV[2])
local windowMs = tonumber(ARGV[3])
local uniquePrefix = ARGV[4]

local t = redis.call('TIME')
local nowMs = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local minScore = nowMs - windowMs

redis.call('ZREMRANGEBYSCORE', key, '-inf', minScore)
local count = redis.call('ZCARD', key)
if (count + n) > limit then
  return 0
end

for i = 1, n do
  redis.call('ZADD', key, nowMs, uniquePrefix .. ':' .. i)
end
redis.call('PEXPIRE', key, windowMs)
return 1
`
