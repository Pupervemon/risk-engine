package redis

import (
	"context"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	goredis "github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	Rdb *goredis.Client
}

func NewRateLimiter(rdb *goredis.Client) *RateLimiter {
	return &RateLimiter{Rdb: rdb}
}

func (r *RateLimiter) IncrementIP(ctx context.Context, ip string, rule domain.RateLimitRule) (domain.RateLimitResult, error) {
	return r.increment(ctx, ipRateLimitKey(ip), rule)
}

func (r *RateLimiter) IncrementUser(ctx context.Context, userID string, scope string, rule domain.RateLimitRule) (domain.RateLimitResult, error) {
	return r.increment(ctx, userRateLimitKey(userID, scope), rule)
}

func (r *RateLimiter) increment(ctx context.Context, key string, rule domain.RateLimitRule) (domain.RateLimitResult, error) {
	val, err := rateLimitLua.Run(ctx, r.Rdb, []string{key}, rule.WindowSeconds).Int64()
	if err != nil {
		return domain.RateLimitResult{}, err
	}

	return domain.RateLimitResult{
		Count:    val,
		Exceeded: val > int64(rule.Limit),
	}, nil
}
