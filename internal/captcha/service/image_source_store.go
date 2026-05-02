package service

import (
	"context"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// ImageSourceStore defines persistence for runtime image-source config.
type ImageSourceStore interface {
	Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error)
	Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error
}

// ImageSourceStorePortAdapter bridges the service-local model to the
// application outbound port during the image-source migration.
type ImageSourceStorePortAdapter struct {
	store appports.RuntimeImageSourceStore
}

var _ ImageSourceStore = (*ImageSourceStorePortAdapter)(nil)

func NewImageSourceStorePortAdapter(store appports.RuntimeImageSourceStore) ImageSourceStore {
	if store == nil {
		return nil
	}
	return &ImageSourceStorePortAdapter{store: store}
}

func (a *ImageSourceStorePortAdapter) Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error) {
	if a == nil || a.store == nil {
		return ImageSourceRuntimeConfig{}, false, nil
	}

	cfg, found, err := a.store.Load(ctx)
	if err != nil {
		return ImageSourceRuntimeConfig{}, false, err
	}
	if !found {
		return ImageSourceRuntimeConfig{}, false, nil
	}

	return ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}, true, nil
}

func (a *ImageSourceStorePortAdapter) Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error {
	if a == nil || a.store == nil {
		return nil
	}

	return a.store.Save(ctx, domain.ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	})
}
