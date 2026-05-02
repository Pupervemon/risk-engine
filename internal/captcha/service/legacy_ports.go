package service

import (
	"context"
	"errors"
	"fmt"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

var (
	_ appports.CaptchaUseCase     = (*CaptchaUseCaseAdapter)(nil)
	_ appports.TokenUseCase       = (*TokenUseCaseAdapter)(nil)
	_ appports.ImageSourceUseCase = (*ImageSourceUseCaseAdapter)(nil)
)

// CaptchaUseCaseAdapter exposes the current CaptchaService through the new
// application port. It is a temporary bridge while the service package is
// decomposed into domain, application, and outbound adapters.
type CaptchaUseCaseAdapter struct {
	service *CaptchaService
}

func NewCaptchaUseCaseAdapter(service *CaptchaService) *CaptchaUseCaseAdapter {
	return &CaptchaUseCaseAdapter{service: service}
}

func (a *CaptchaUseCaseAdapter) Generate(ctx context.Context) (domain.SliderChallenge, error) {
	if a == nil || a.service == nil {
		return domain.SliderChallenge{}, fmt.Errorf("captcha service is not configured")
	}

	challenge, err := a.service.Generate(ctx)
	if err != nil {
		return domain.SliderChallenge{}, err
	}
	if challenge == nil {
		return domain.SliderChallenge{}, fmt.Errorf("captcha challenge is empty")
	}

	return domain.SliderChallenge{
		CaptchaID:         challenge.CaptchaID,
		MasterImage:       challenge.MasterImage,
		TileImage:         challenge.TileImage,
		TargetY:           challenge.TargetY,
		ExpiresIn:         challenge.ExpiresIn,
		RequireMouseTrack: challenge.RequireMouseTrack,
	}, nil
}

func (a *CaptchaUseCaseAdapter) Verify(ctx context.Context, cmd appports.VerifyCaptchaCommand) (appports.VerifyCaptchaResult, error) {
	if a == nil || a.service == nil {
		return appports.VerifyCaptchaResult{}, fmt.Errorf("captcha service is not configured")
	}

	var mouseTrack *[]TrackPoint
	if cmd.MouseTrackProvided {
		converted := make([]TrackPoint, 0, len(cmd.MouseTrack))
		for _, point := range cmd.MouseTrack {
			converted = append(converted, TrackPoint{
				X:    point.X,
				Y:    point.Y,
				Time: point.Time,
			})
		}
		mouseTrack = &converted
	}

	valid, reason, err := a.service.VerifyWithTrack(ctx, cmd.CaptchaID, cmd.PointX, cmd.PointY, mouseTrack)
	return appports.VerifyCaptchaResult{
		Valid:  valid,
		Reason: reason,
	}, err
}

// TokenUseCaseAdapter exposes the current TokenService through the new
// application port.
type TokenUseCaseAdapter struct {
	service *TokenService
}

func NewTokenUseCaseAdapter(service *TokenService) *TokenUseCaseAdapter {
	return &TokenUseCaseAdapter{service: service}
}

func (a *TokenUseCaseAdapter) Issue(ctx context.Context, captchaID string) (appports.IssuedToken, error) {
	if a == nil || a.service == nil {
		return appports.IssuedToken{}, fmt.Errorf("token service is not configured")
	}

	token, expiresAt, err := a.service.IssueToken(ctx, captchaID)
	if err != nil {
		return appports.IssuedToken{}, err
	}

	return appports.IssuedToken{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (a *TokenUseCaseAdapter) Verify(ctx context.Context, token string) (appports.TokenVerification, error) {
	if a == nil || a.service == nil {
		return appports.TokenVerification{}, fmt.Errorf("token service is not configured")
	}

	valid, reason, expiresAt := a.service.VerifyToken(ctx, token)
	return appports.TokenVerification{
		Valid:     valid,
		Reason:    reason,
		ExpiresAt: expiresAt,
	}, nil
}

// ImageSourceUseCaseAdapter exposes runtime image-source management through the
// new application port.
type ImageSourceUseCaseAdapter struct {
	service *CaptchaService
}

func NewImageSourceUseCaseAdapter(service *CaptchaService) *ImageSourceUseCaseAdapter {
	return &ImageSourceUseCaseAdapter{service: service}
}

func (a *ImageSourceUseCaseAdapter) Status(ctx context.Context) (domain.ImageSourceStatus, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceStatus{}, fmt.Errorf("captcha service is not configured")
	}

	status, err := a.service.GetImageSourceStatus(ctx)
	return imageSourceStatusToDomain(status), mapImageSourceError(err)
}

func (a *ImageSourceUseCaseAdapter) Validate(ctx context.Context, patch domain.ImageSourcePatch) (domain.ImageSourceValidationResult, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceValidationResult{}, fmt.Errorf("captcha service is not configured")
	}

	result, err := a.service.ValidateImageSource(ctx, imageSourcePatchFromDomain(patch))
	return imageSourceValidationResultToDomain(result), mapImageSourceError(err)
}

func (a *ImageSourceUseCaseAdapter) Update(ctx context.Context, patch domain.ImageSourcePatch, triggerRefresh bool) (domain.ImageSourceStatus, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceStatus{}, fmt.Errorf("captcha service is not configured")
	}

	status, err := a.service.UpdateImageSource(ctx, imageSourcePatchFromDomain(patch), triggerRefresh)
	return imageSourceStatusToDomain(status), mapImageSourceError(err)
}

func (a *ImageSourceUseCaseAdapter) Refresh(ctx context.Context) (domain.ImageSourceStatus, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceStatus{}, fmt.Errorf("captcha service is not configured")
	}

	status, err := a.service.RefreshImagePool(ctx)
	return imageSourceStatusToDomain(status), mapImageSourceError(err)
}

func imageSourcePatchFromDomain(patch domain.ImageSourcePatch) ImageSourcePatch {
	return ImageSourcePatch{
		URL:                patch.URL,
		APIKey:             patch.APIKey,
		TimeoutSeconds:     patch.TimeoutSeconds,
		RateLimitPerMinute: patch.RateLimitPerMinute,
		RetryCount:         patch.RetryCount,
	}
}

func imageSourceStatusToDomain(status ImageSourceStatus) domain.ImageSourceStatus {
	return domain.ImageSourceStatus{
		Enabled:             status.Enabled,
		Version:             status.Version,
		Config:              imageSourceConfigViewToDomain(status.Config),
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

func imageSourceValidationResultToDomain(result ImageSourceValidationResult) domain.ImageSourceValidationResult {
	return domain.ImageSourceValidationResult{
		Config:      imageSourceConfigViewToDomain(result.Config),
		ValidatedAt: result.ValidatedAt,
	}
}

func imageSourceConfigViewToDomain(config ImageSourceConfigView) domain.ImageSourceConfigView {
	return domain.ImageSourceConfigView{
		URL:                config.URL,
		APIKeyConfigured:   config.APIKeyConfigured,
		TimeoutSeconds:     config.TimeoutSeconds,
		RateLimitPerMinute: config.RateLimitPerMinute,
		RetryCount:         config.RetryCount,
	}
}

func mapImageSourceError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrImagePoolDisabled) {
		return domain.ErrImagePoolDisabled
	}
	if errors.Is(err, ErrImagePoolRefreshInProgress) {
		return domain.ErrImagePoolRefreshInProgress
	}

	var refreshErr *ImageSourceRefreshError
	if errors.As(err, &refreshErr) {
		return &domain.ImageSourceRefreshError{Err: refreshErr.Err}
	}

	var persistenceErr *ImageSourcePersistenceError
	if errors.As(err, &persistenceErr) {
		return &domain.ImageSourcePersistenceError{Err: persistenceErr.Err}
	}

	return err
}
