package redis

import (
	"context"
	"fmt"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	goredis "github.com/redis/go-redis/v9"
)

type LoginFailureRepository struct {
	Rdb *goredis.Client
}

func NewLoginFailureRepository(rdb *goredis.Client) *LoginFailureRepository {
	return &LoginFailureRepository{Rdb: rdb}
}

func (r *LoginFailureRepository) GetByUserID(ctx context.Context, userID string) (int64, error) {
	return r.Rdb.Get(ctx, loginFailCountKeyByUserID(userID)).Int64()
}

func (r *LoginFailureRepository) GetByIP(ctx context.Context, ip string) (int64, error) {
	return r.Rdb.Get(ctx, loginFailCountKeyByIP(ip)).Int64()
}

func (r *LoginFailureRepository) RecordFailure(ctx context.Context, target domain.LoginFailureTarget, expireSeconds int) (int64, error) {
	if expireSeconds <= 0 {
		return 0, fmt.Errorf("invalid fail_count_expire_minutes: %d", expireSeconds/60)
	}

	return loginFailCountLua.Run(ctx, r.Rdb, []string{loginFailCountKey(target)}, expireSeconds).Int64()
}

func (r *LoginFailureRepository) Clear(ctx context.Context, target domain.LoginFailureTarget) error {
	return r.Rdb.Del(ctx, loginFailCountKey(target)).Err()
}
