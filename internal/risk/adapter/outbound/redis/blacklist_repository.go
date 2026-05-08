package redis

import (
	"context"
	"errors"
	"time"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	goredis "github.com/redis/go-redis/v9"
)

type BlacklistRepository struct {
	Rdb *goredis.Client
}

func NewBlacklistRepository(rdb *goredis.Client) *BlacklistRepository {
	return &BlacklistRepository{Rdb: rdb}
}

func (r *BlacklistRepository) Get(ctx context.Context, entryType domain.BlacklistType, value string) (domain.BlacklistEntry, bool, error) {
	reason, err := r.Rdb.Get(ctx, blacklistKey(entryType, value)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return domain.BlacklistEntry{}, false, nil
		}
		return domain.BlacklistEntry{}, false, err
	}
	return domain.BlacklistEntry{Type: entryType, Value: value, Reason: reason}, true, nil
}

func (r *BlacklistRepository) Save(ctx context.Context, entry domain.BlacklistEntry) error {
	var expiration time.Duration
	if entry.ExpireAt > 0 {
		expiration = time.Until(time.Unix(entry.ExpireAt, 0))
		if expiration < 0 {
			expiration = time.Second
		}
	}

	return r.Rdb.Set(ctx, blacklistKey(entry.Type, entry.Value), entry.Reason, expiration).Err()
}
