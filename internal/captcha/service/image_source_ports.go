package service

import (
	"context"
	"fmt"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type RuntimeImageSourceManagerPortAdapter struct {
	manager *RuntimeImageSourceManager
}

var _ appports.RuntimeImageSourceManager = (*RuntimeImageSourceManagerPortAdapter)(nil)

func NewRuntimeImageSourceManagerPortAdapter(manager *RuntimeImageSourceManager) *RuntimeImageSourceManagerPortAdapter {
	if manager == nil {
		return nil
	}
	return &RuntimeImageSourceManagerPortAdapter{manager: manager}
}

func (a *RuntimeImageSourceManagerPortAdapter) BuildCandidateConfig(patch domain.ImageSourcePatch) (domain.ImageSourceRuntimeConfig, error) {
	if a == nil || a.manager == nil {
		return domain.ImageSourceRuntimeConfig{}, fmt.Errorf("runtime image source manager is not configured")
	}

	cfg, err := a.manager.BuildCandidateConfig(imageSourcePatchFromDomain(patch))
	if err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}
	return imageSourceRuntimeConfigToDomain(cfg), nil
}

func (a *RuntimeImageSourceManagerPortAdapter) ValidateConfig(ctx context.Context, candidate domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	if a == nil || a.manager == nil {
		return nil, fmt.Errorf("runtime image source manager is not configured")
	}

	provider, err := a.manager.ValidateConfig(ctx, imageSourceRuntimeConfigFromDomain(candidate))
	if err != nil {
		return nil, err
	}
	return serviceImageProviderToAppAdapter{provider: provider}, nil
}

func (a *RuntimeImageSourceManagerPortAdapter) ApplyConfig(candidate domain.ImageSourceRuntimeConfig, provider appports.ImageProvider) {
	if a == nil || a.manager == nil {
		return
	}
	a.manager.ApplyConfig(
		imageSourceRuntimeConfigFromDomain(candidate),
		serviceImageProviderAdapter{provider: provider},
	)
}

func (a *RuntimeImageSourceManagerPortAdapter) RecordRefreshResult(err error) {
	if a != nil && a.manager != nil {
		a.manager.RecordRefreshResult(err)
	}
}

func (a *RuntimeImageSourceManagerPortAdapter) ValidationResult(candidate domain.ImageSourceRuntimeConfig) domain.ImageSourceValidationResult {
	if a == nil || a.manager == nil {
		return domain.ImageSourceValidationResult{}
	}
	return imageSourceValidationResultToDomain(a.manager.ValidationResult(imageSourceRuntimeConfigFromDomain(candidate)))
}

func (a *RuntimeImageSourceManagerPortAdapter) Status(poolSize int, poolSnapshot domain.ImagePoolSnapshot) domain.ImageSourceStatus {
	if a == nil || a.manager == nil {
		return domain.ImageSourceStatus{Enabled: false}
	}

	return imageSourceStatusToDomain(a.manager.Status(poolSize, ImagePoolSnapshot{
		ImageCount:       poolSnapshot.ImageCount,
		ActiveGeneration: poolSnapshot.ActiveGeneration,
		GenerationCount:  poolSnapshot.GenerationCount,
	}))
}

type serviceImageProviderToAppAdapter struct {
	provider ImageProvider
}

var _ appports.ImageProvider = serviceImageProviderToAppAdapter{}

func (a serviceImageProviderToAppAdapter) FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("image provider is not configured")
	}

	images, err := a.provider.FetchImages(ctx, count)
	if err != nil {
		return nil, err
	}
	return imageMetasToDomain(images), nil
}

func imageSourceRuntimeConfigToDomain(cfg ImageSourceRuntimeConfig) domain.ImageSourceRuntimeConfig {
	return domain.ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func imageSourceRuntimeConfigFromDomain(cfg domain.ImageSourceRuntimeConfig) ImageSourceRuntimeConfig {
	return ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}
