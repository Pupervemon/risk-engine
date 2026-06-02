package application

import (
	"context"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type ImageSourceOptions struct {
	PoolSize int
}

type ImageSourceUseCase struct {
	pool            appports.BackgroundImagePool
	store           appports.RuntimeImageSourceStore
	providerFactory appports.ImageProviderFactory
	opts            ImageSourceOptions
}

var _ appports.ImageSourceUseCase = (*ImageSourceUseCase)(nil)

func NewImageSourceUseCase(
	pool appports.BackgroundImagePool,
	store appports.RuntimeImageSourceStore,
	providerFactory appports.ImageProviderFactory,
	opts ImageSourceOptions,
) *ImageSourceUseCase {
	return &ImageSourceUseCase{
		pool:            pool,
		store:           store,
		providerFactory: providerFactory,
		opts:            opts,
	}
}

func (u *ImageSourceUseCase) Status(ctx context.Context) (domain.ImageSourceStatus, error) {
	if u == nil || u.pool == nil || u.store == nil {
		return domain.ImageSourceStatus{Enabled: false}, nil
	}

	cfg, found, err := u.store.Load(ctx)
	if err != nil {
		return domain.ImageSourceStatus{}, err
	}
	if !found {
		return domain.ImageSourceStatus{Enabled: true}, nil
	}

	runtimeStatus, err := u.store.LoadStatus(ctx)
	if err != nil {
		return domain.ImageSourceStatus{}, err
	}

	poolSnapshot, err := u.pool.Snapshot(ctx)
	if err != nil {
		poolSnapshot = domain.ImagePoolSnapshot{}
	}

	return buildImageSourceStatus(cfg, runtimeStatus, u.opts.PoolSize, poolSnapshot), nil
}

func (u *ImageSourceUseCase) Check(ctx context.Context) (domain.ImageSourceValidationResult, error) {
	cfg, err := u.currentConfig(ctx)
	if err != nil {
		return domain.ImageSourceValidationResult{}, err
	}

	if _, err := u.validateConfig(ctx, cfg); err != nil {
		_ = u.recordValidation(ctx, err)
		return domain.ImageSourceValidationResult{}, err
	}

	if err := u.recordValidation(ctx, nil); err != nil {
		return domain.ImageSourceValidationResult{}, err
	}

	return domain.ImageSourceValidationResult{
		Config:      ImageSourceConfigPublicView(cfg),
		ValidatedAt: time.Now().Format(time.RFC3339),
	}, nil
}

func (u *ImageSourceUseCase) Update(ctx context.Context, patch domain.ImageSourcePatch, triggerRefresh bool) (domain.ImageSourceStatus, error) {
	current, err := u.currentConfig(ctx)
	if err != nil {
		status, _ := u.Status(ctx)
		return status, err
	}

	candidate, err := BuildImageSourceCandidateConfig(current, patch)
	if err != nil {
		_ = u.recordValidation(ctx, err)
		status, _ := u.Status(ctx)
		return status, err
	}

	if _, err := u.validateConfig(ctx, candidate); err != nil {
		_ = u.recordValidation(ctx, err)
		status, _ := u.Status(ctx)
		return status, err
	}

	now := time.Now().Format(time.RFC3339)
	candidate.Version = current.Version + 1
	if candidate.Version <= 0 {
		candidate.Version = 1
	}
	candidate.UpdatedAt = now

	if err := u.store.Save(ctx, candidate); err != nil {
		status, _ := u.Status(ctx)
		return status, &domain.ImageSourcePersistenceError{Err: err}
	}
	if err := u.recordValidation(ctx, nil); err != nil {
		status, _ := u.Status(ctx)
		return status, &domain.ImageSourcePersistenceError{Err: err}
	}

	if triggerRefresh {
		if status, err := u.Refresh(ctx); err != nil {
			return status, err
		}
	}

	return u.Status(ctx)
}

func (u *ImageSourceUseCase) Refresh(ctx context.Context) (domain.ImageSourceStatus, error) {
	cfg, err := u.currentConfig(ctx)
	if err != nil {
		status, _ := u.Status(ctx)
		return status, err
	}

	provider, err := u.buildProvider(cfg)
	if err != nil {
		status, _ := u.Status(ctx)
		return status, err
	}

	meta := domain.ImagePoolGenerationMeta{
		SourceConfigVersion: cfg.Version,
		SourceURL:           cfg.URL,
	}

	err = u.pool.RefreshWithProvider(ctx, provider, meta)
	_ = u.recordRefresh(ctx, err)

	status, _ := u.Status(ctx)
	if err != nil {
		return status, &domain.ImageSourceRefreshError{Err: err}
	}

	return status, nil
}

func (u *ImageSourceUseCase) currentConfig(ctx context.Context) (domain.ImageSourceRuntimeConfig, error) {
	if u == nil || u.pool == nil || u.store == nil {
		return domain.ImageSourceRuntimeConfig{}, domain.ErrImagePoolDisabled
	}

	cfg, found, err := u.store.Load(ctx)
	if err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}
	if !found {
		return domain.ImageSourceRuntimeConfig{}, fmt.Errorf("image source config is missing")
	}
	if err := ValidateImageSourceRuntimeConfig(cfg); err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}
	return cfg, nil
}

func (u *ImageSourceUseCase) validateConfig(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	provider, err := u.buildProvider(cfg)
	if err == nil {
		var images []domain.ImageMeta
		images, err = provider.FetchImages(ctx, 1)
		if err == nil && len(images) == 0 {
			err = fmt.Errorf("validation fetched zero images")
		}
	}
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (u *ImageSourceUseCase) buildProvider(cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	if u == nil || u.providerFactory == nil {
		return nil, fmt.Errorf("image provider factory is not configured")
	}
	if err := ValidateImageSourceRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	return u.providerFactory.BuildRuntimeProvider(cfg)
}

func (u *ImageSourceUseCase) recordValidation(ctx context.Context, err error) error {
	status, loadErr := u.store.LoadStatus(ctx)
	if loadErr != nil {
		return loadErr
	}

	status.LastValidatedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		status.LastValidationError = err.Error()
	} else {
		status.LastValidationError = ""
	}
	return u.store.SaveStatus(ctx, status)
}

func (u *ImageSourceUseCase) recordRefresh(ctx context.Context, err error) error {
	status, loadErr := u.store.LoadStatus(ctx)
	if loadErr != nil {
		return loadErr
	}

	status.LastRefreshedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		status.LastRefreshError = err.Error()
	} else {
		status.LastRefreshError = ""
	}
	return u.store.SaveStatus(ctx, status)
}

func buildImageSourceStatus(
	cfg domain.ImageSourceRuntimeConfig,
	runtimeStatus domain.ImageSourceRuntimeStatus,
	poolSize int,
	poolSnapshot domain.ImagePoolSnapshot,
) domain.ImageSourceStatus {
	synced := cfg.Version > 0 &&
		poolSnapshot.SourceConfigVersion > 0 &&
		cfg.Version == poolSnapshot.SourceConfigVersion

	message := "image pool is using current image source config"
	if !synced {
		message = "image source config has changed but image pool has not been refreshed"
	}
	if poolSnapshot.SourceConfigVersion == 0 {
		message = "image pool has not been refreshed with an image source config"
	}

	return domain.ImageSourceStatus{
		Enabled: true,
		Config:  ImageSourceConfigPublicView(cfg),
		ActivePool: domain.ImageSourceActivePoolView{
			SourceConfigVersion: poolSnapshot.SourceConfigVersion,
			SourceURL:           poolSnapshot.SourceURL,
			ImageCount:          poolSnapshot.ImageCount,
			RefreshedAt:         poolSnapshot.RefreshedAt,
		},
		Sync: domain.ImageSourceSyncStatus{
			PoolSyncedWithConfig: synced,
			Message:              message,
		},
		UpdatedAt:           cfg.UpdatedAt,
		LastValidatedAt:     runtimeStatus.LastValidatedAt,
		LastValidationError: runtimeStatus.LastValidationError,
		LastRefreshedAt:     runtimeStatus.LastRefreshedAt,
		LastRefreshError:    runtimeStatus.LastRefreshError,
		PoolSize:            poolSize,
		PoolImageCount:      poolSnapshot.ImageCount,
		ActiveGeneration:    poolSnapshot.ActiveGeneration,
		GenerationCount:     poolSnapshot.GenerationCount,
	}
}
