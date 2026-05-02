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
	return domain.ImagePoolSnapshot{
		ImageCount:       snapshot.ImageCount,
		ActiveGeneration: snapshot.ActiveGeneration,
		GenerationCount:  snapshot.GenerationCount,
	}, nil
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
	return a.pool.RefreshWithProvider(ctx, serviceImageProviderAdapter{provider: provider})
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

type serviceImageProviderAdapter struct {
	provider appports.ImageProvider
}

var _ ImageProvider = serviceImageProviderAdapter{}

func (a serviceImageProviderAdapter) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("image provider is not configured")
	}

	images, err := a.provider.FetchImages(ctx, count)
	if err != nil {
		return nil, err
	}

	converted := make([]ImageMeta, 0, len(images))
	for _, image := range images {
		converted = append(converted, ImageMeta{
			ID:   image.ID,
			Data: image.Data,
			URL:  image.URL,
		})
	}

	return converted, nil
}
