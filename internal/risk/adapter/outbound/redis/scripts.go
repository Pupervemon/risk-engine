package redis

import goredis "github.com/redis/go-redis/v9"

var rateLimitLua = goredis.NewScript(`
    local current = redis.call("INCR", KEYS[1])
    if current == 1 then
        redis.call("EXPIRE", KEYS[1], ARGV[1])
    end
    return current
`)

var loginFailCountLua = goredis.NewScript(`
    local current = redis.call("INCR", KEYS[1])
    redis.call("EXPIRE", KEYS[1], ARGV[1])
    return current
`)
