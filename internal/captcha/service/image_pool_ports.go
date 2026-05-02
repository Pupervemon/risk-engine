package service

import (
	"context"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// ImagePoolPortAdapter exposes the current RedisImagePool through the
// application port while image-pool storage is migrated out of service.
type ImagePoolPortAdapter struct {
	pool *RedisImagePool
}

var _ appports.BackgroundImagePool = (*ImagePoolPortAdapter)(nil)

func NewImagePoolPortAdapter(pool *RedisImagePool) *ImagePoolPortAdapter {
	if pool == nil {
		return nil
	}
	return &ImagePoolPortAdapter{pool: pool}
}

func (a *ImagePoolPortAdapter) Random(ctx context.Context) ([]byte, error) {
	if a == nil || a.pool == nil {
		return nil, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.GetRandom(ctx)
}

func (a *ImagePoolPortAdapter) Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error) {
	if a == nil || a.pool == nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("captcha image pool is not configured")
	}

	snapshot, err := a.pool.Snapshot(ctx)
	if err != nil {
		return domain.ImagePoolSnapshot{}, err
	}
	return snapshot, nil
}

func (a *ImagePoolPortAdapter) Refresh(ctx context.Context) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshNow(ctx)
}

func (a *ImagePoolPortAdapter) RefreshWithProvider(ctx context.Context, provider appports.ImageProvider) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshWithProvider(ctx, provider)
}

func (a *ImagePoolPortAdapter) Start(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if a != nil && a.pool != nil {
		a.pool.StartRefresh(ctx, interval, refreshOnStartup)
	}
}

func (a *ImagePoolPortAdapter) Stop() {
	if a != nil && a.pool != nil {
		a.pool.StopRefresh()
	}
}
