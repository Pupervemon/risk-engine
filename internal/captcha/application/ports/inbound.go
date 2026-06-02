package ports

import (
	"context"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type CaptchaUseCase interface {
	Generate(ctx context.Context) (domain.SliderChallenge, error)
	Verify(ctx context.Context, cmd VerifyCaptchaCommand) (VerifyCaptchaResult, error)
}

type VerifyCaptchaCommand struct {
	CaptchaID          string
	PointX             int
	PointY             int
	MouseTrack         []domain.TrackPoint
	MouseTrackProvided bool
}

type VerifyCaptchaResult struct {
	Valid  bool
	Reason string
}

type TokenUseCase interface {
	Issue(ctx context.Context, captchaID string) (IssuedToken, error)
	Verify(ctx context.Context, token string) (TokenVerification, error)
}

type IssuedToken struct {
	Token     string
	ExpiresAt int64
}

type TokenVerification struct {
	Valid     bool
	Reason    string
	ExpiresAt int64
}

type ImageSourceUseCase interface {
	Status(ctx context.Context) (domain.ImageSourceStatus, error)
	Check(ctx context.Context) (domain.ImageSourceValidationResult, error)
	Update(ctx context.Context, patch domain.ImageSourcePatch, triggerRefresh bool) (domain.ImageSourceStatus, error)
	Refresh(ctx context.Context) (domain.ImageSourceStatus, error)
}

type CaptchaLifecycle interface {
	StartImageRefresh(ctx context.Context) error
	StopImageRefresh()
}
