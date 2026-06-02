package imagepool

import (
	"context"
	"fmt"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type PortAdapter struct {
	pool *RedisImagePool
}

var _ appports.BackgroundImagePool = (*PortAdapter)(nil)

func NewPortAdapter(pool *RedisImagePool) *PortAdapter {
	if pool == nil {
		return nil
	}
	return &PortAdapter{pool: pool}
}

func (a *PortAdapter) Random(ctx context.Context) ([]byte, error) {
	if a == nil || a.pool == nil {
		return nil, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.GetRandom(ctx)
}

func (a *PortAdapter) Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error) {
	if a == nil || a.pool == nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.Snapshot(ctx)
}

func (a *PortAdapter) RefreshWithProvider(ctx context.Context, provider appports.ImageProvider, meta domain.ImagePoolGenerationMeta) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshWithProvider(ctx, provider, meta)
}
