package service

import (
	"context"
	"errors"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// GetImageSourceStatus returns the current runtime image source status.
func (s *CaptchaService) GetImageSourceStatus(ctx context.Context) (ImageSourceStatus, error) {
	if s == nil || s.imageSourceUseCase == nil {
		return ImageSourceStatus{Enabled: false}, nil
	}

	status, err := s.imageSourceUseCase.Status(ctx)
	if err != nil {
		return ImageSourceStatus{}, mapDomainImageSourceError(err)
	}
	return imageSourceStatusFromDomain(status), nil
}

// ValidateImageSource validates a candidate image source config without applying it.
func (s *CaptchaService) ValidateImageSource(ctx context.Context, patch ImageSourcePatch) (ImageSourceValidationResult, error) {
	if s == nil || s.imageSourceUseCase == nil {
		return ImageSourceValidationResult{}, ErrImagePoolDisabled
	}

	result, err := s.imageSourceUseCase.Validate(ctx, imageSourcePatchToDomain(patch))
	return imageSourceValidationResultFromDomain(result), mapDomainImageSourceError(err)
}

// UpdateImageSource validates, persists, and applies a new runtime image source config.
func (s *CaptchaService) UpdateImageSource(ctx context.Context, patch ImageSourcePatch, triggerRefresh bool) (ImageSourceStatus, error) {
	if s == nil || s.imageSourceUseCase == nil {
		return ImageSourceStatus{}, ErrImagePoolDisabled
	}

	status, err := s.imageSourceUseCase.Update(ctx, imageSourcePatchToDomain(patch), triggerRefresh)
	return imageSourceStatusFromDomain(status), mapDomainImageSourceError(err)
}

// RefreshImagePool refreshes the image pool with the currently active runtime config.
func (s *CaptchaService) RefreshImagePool(ctx context.Context) (ImageSourceStatus, error) {
	if s == nil || s.imageSourceUseCase == nil {
		return ImageSourceStatus{}, ErrImagePoolDisabled
	}

	status, err := s.imageSourceUseCase.Refresh(ctx)
	return imageSourceStatusFromDomain(status), mapDomainImageSourceError(err)
}

func imageSourcePatchToDomain(patch ImageSourcePatch) domain.ImageSourcePatch {
	return domain.ImageSourcePatch{
		URL:                patch.URL,
		APIKey:             patch.APIKey,
		TimeoutSeconds:     patch.TimeoutSeconds,
		RateLimitPerMinute: patch.RateLimitPerMinute,
		RetryCount:         patch.RetryCount,
	}
}

func imageSourceStatusFromDomain(status domain.ImageSourceStatus) ImageSourceStatus {
	return ImageSourceStatus{
		Enabled:             status.Enabled,
		Version:             status.Version,
		Config:              imageSourceConfigViewFromDomain(status.Config),
		UpdatedAt:           status.UpdatedAt,
		LastValidatedAt:     status.LastValidatedAt,
		LastValidationError: status.LastValidationError,
		LastRefreshedAt:     status.LastRefreshedAt,
		LastRefreshError:    status.LastRefreshError,
		PoolSize:            status.PoolSize,
		PoolImageCount:      status.PoolImageCount,
		ActiveGeneration:    status.ActiveGeneration,
		GenerationCount:     status.GenerationCount,
	}
}

func imageSourceValidationResultFromDomain(result domain.ImageSourceValidationResult) ImageSourceValidationResult {
	return ImageSourceValidationResult{
		Config:      imageSourceConfigViewFromDomain(result.Config),
		ValidatedAt: result.ValidatedAt,
	}
}

func imageSourceConfigViewFromDomain(config domain.ImageSourceConfigView) ImageSourceConfigView {
	return ImageSourceConfigView{
		URL:                config.URL,
		APIKeyConfigured:   config.APIKeyConfigured,
		TimeoutSeconds:     config.TimeoutSeconds,
		RateLimitPerMinute: config.RateLimitPerMinute,
		RetryCount:         config.RetryCount,
	}
}

func mapDomainImageSourceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrImagePoolDisabled) {
		return ErrImagePoolDisabled
	}
	if errors.Is(err, domain.ErrImagePoolRefreshInProgress) {
		return ErrImagePoolRefreshInProgress
	}

	var refreshErr *domain.ImageSourceRefreshError
	if errors.As(err, &refreshErr) {
		return &ImageSourceRefreshError{Err: refreshErr.Err}
	}

	var persistenceErr *domain.ImageSourcePersistenceError
	if errors.As(err, &persistenceErr) {
		return &ImageSourcePersistenceError{Err: persistenceErr.Err}
	}

	return err
}
