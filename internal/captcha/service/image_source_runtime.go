package service

import (
	"context"

	"go.uber.org/zap"
)

// GetImageSourceStatus returns the current runtime image source status.
func (s *CaptchaService) GetImageSourceStatus(ctx context.Context) (ImageSourceStatus, error) {
	manager := s.runtimeImageSourceManager()
	if manager == nil || s.imagePool == nil {
		return ImageSourceStatus{Enabled: false}, nil
	}

	poolSnapshot, err := s.imagePool.Snapshot(ctx)
	if err != nil {
		s.logger.Warn("failed to inspect image pool contents", zap.Error(err))
		poolSnapshot = ImagePoolSnapshot{}
	}

	return manager.Status(s.imagePool.poolSize, poolSnapshot), nil
}

// ValidateImageSource validates a candidate image source config without applying it.
func (s *CaptchaService) ValidateImageSource(ctx context.Context, patch ImageSourcePatch) (ImageSourceValidationResult, error) {
	manager := s.runtimeImageSourceManager()
	if manager == nil || s.imagePool == nil {
		return ImageSourceValidationResult{}, ErrImagePoolDisabled
	}

	candidate, err := manager.BuildCandidateConfig(patch)
	if err != nil {
		return ImageSourceValidationResult{}, err
	}

	if _, err := manager.ValidateConfig(ctx, candidate); err != nil {
		return ImageSourceValidationResult{}, err
	}

	return manager.ValidationResult(candidate), nil
}

// UpdateImageSource validates, persists, and applies a new runtime image source config.
func (s *CaptchaService) UpdateImageSource(ctx context.Context, patch ImageSourcePatch, triggerRefresh bool) (ImageSourceStatus, error) {
	manager := s.runtimeImageSourceManager()
	if manager == nil || s.imagePool == nil {
		return ImageSourceStatus{}, ErrImagePoolDisabled
	}

	candidate, err := manager.BuildCandidateConfig(patch)
	if err != nil {
		return ImageSourceStatus{}, err
	}

	provider, err := manager.ValidateConfig(ctx, candidate)
	if err != nil {
		return ImageSourceStatus{}, err
	}

	if err := s.persistRuntimeImageSourceConfig(ctx, candidate); err != nil {
		status, _ := s.GetImageSourceStatus(ctx)
		return status, &ImageSourcePersistenceError{Err: err}
	}

	manager.ApplyConfig(candidate, provider)

	if triggerRefresh {
		err = s.imagePool.RefreshWithProvider(ctx, provider)
		manager.RecordRefreshResult(err)

		status, _ := s.GetImageSourceStatus(ctx)
		if err != nil {
			return status, &ImageSourceRefreshError{Err: err}
		}

		return status, nil
	}

	return s.GetImageSourceStatus(ctx)
}

// RefreshImagePool refreshes the image pool with the currently active runtime config.
func (s *CaptchaService) RefreshImagePool(ctx context.Context) (ImageSourceStatus, error) {
	manager := s.runtimeImageSourceManager()
	if manager == nil || s.imagePool == nil {
		return ImageSourceStatus{}, ErrImagePoolDisabled
	}

	err := s.imagePool.RefreshNow(ctx)
	manager.RecordRefreshResult(err)

	status, _ := s.GetImageSourceStatus(ctx)
	if err != nil {
		return status, &ImageSourceRefreshError{Err: err}
	}

	return status, nil
}

func (s *CaptchaService) persistRuntimeImageSourceConfig(ctx context.Context, cfg ImageSourceRuntimeConfig) error {
	store := s.runtimeImageSourceStore()
	if store == nil {
		return nil
	}

	if err := store.Save(ctx, cfg); err != nil {
		s.logger.Error("failed to persist runtime image source config", zap.Error(err))
		return err
	}

	return nil
}
