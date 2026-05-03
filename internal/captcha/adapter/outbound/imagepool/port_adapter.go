package imagepool

import (
	"context"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// PortAdapter exposes RedisImagePool through the application image-pool port.
type PortAdapter struct {
	pool *RedisImagePool
}

var _ appports.ManagedBackgroundImagePool = (*PortAdapter)(nil)

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

	snapshot, err := a.pool.Snapshot(ctx)
	if err != nil {
		return domain.ImagePoolSnapshot{}, err
	}
	return snapshot, nil
}

func (a *PortAdapter) Count(ctx context.Context) (int64, error) {
	if a == nil || a.pool == nil {
		return 0, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.Count(ctx)
}

func (a *PortAdapter) Refresh(ctx context.Context) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshNow(ctx)
}

func (a *PortAdapter) RefreshWithProvider(ctx context.Context, provider appports.ImageProvider) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshWithProvider(ctx, provider)
}

func (a *PortAdapter) Start(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if a != nil && a.pool != nil {
		a.pool.StartRefresh(ctx, interval, refreshOnStartup)
	}
}

func (a *PortAdapter) Stop() {
	if a != nil && a.pool != nil {
		a.pool.StopRefresh()
	}
}

func (a *PortAdapter) SetProvider(provider appports.ImageProvider) {
	if a != nil && a.pool != nil {
		a.pool.SetProvider(provider)
	}
}

func (a *PortAdapter) PoolSize() int {
	if a == nil || a.pool == nil {
		return 0
	}
	return a.pool.PoolSize()
}
