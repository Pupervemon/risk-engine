package redisadapter

import (
	"context"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/redis/go-redis/v9"
)

const tokenRedisPrefix = "captcha:token:"

// TokenRepository persists token payloads in Redis.
type TokenRepository struct {
	rdb *redis.Client
}

var _ appports.TokenRepository = (*TokenRepository)(nil)

// NewTokenRepository creates a Redis-backed token repository.
func NewTokenRepository(rdb *redis.Client) *TokenRepository {
	if rdb == nil {
		return nil
	}
	return &TokenRepository{rdb: rdb}
}

func (r *TokenRepository) Save(ctx context.Context, tokenDigest string, value []byte, expiration time.Duration) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("token repository is not configured")
	}
	return r.rdb.Set(ctx, tokenKey(tokenDigest), value, expiration).Err()
}

func (r *TokenRepository) Exists(ctx context.Context, tokenDigest string) (bool, error) {
	if r == nil || r.rdb == nil {
		return false, fmt.Errorf("token repository is not configured")
	}

	count, err := r.rdb.Exists(ctx, tokenKey(tokenDigest)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func tokenKey(tokenDigest string) string {
	return tokenRedisPrefix + tokenDigest
}
