package service

import (
	"context"
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
	if a.service.imageSourceUseCase == nil {
		return domain.ImageSourceStatus{Enabled: false}, nil
	}

	return a.service.imageSourceUseCase.Status(ctx)
}

func (a *ImageSourceUseCaseAdapter) Validate(ctx context.Context, patch domain.ImageSourcePatch) (domain.ImageSourceValidationResult, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceValidationResult{}, fmt.Errorf("captcha service is not configured")
	}
	if a.service.imageSourceUseCase == nil {
		return domain.ImageSourceValidationResult{}, domain.ErrImagePoolDisabled
	}

	return a.service.imageSourceUseCase.Validate(ctx, patch)
}

func (a *ImageSourceUseCaseAdapter) Update(ctx context.Context, patch domain.ImageSourcePatch, triggerRefresh bool) (domain.ImageSourceStatus, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceStatus{}, fmt.Errorf("captcha service is not configured")
	}
	if a.service.imageSourceUseCase == nil {
		return domain.ImageSourceStatus{}, domain.ErrImagePoolDisabled
	}

	return a.service.imageSourceUseCase.Update(ctx, patch, triggerRefresh)
}

func (a *ImageSourceUseCaseAdapter) Refresh(ctx context.Context) (domain.ImageSourceStatus, error) {
	if a == nil || a.service == nil {
		return domain.ImageSourceStatus{}, fmt.Errorf("captcha service is not configured")
	}
	if a.service.imageSourceUseCase == nil {
		return domain.ImageSourceStatus{}, domain.ErrImagePoolDisabled
	}

	return a.service.imageSourceUseCase.Refresh(ctx)
}
