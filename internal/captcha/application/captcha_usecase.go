package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// CaptchaOptions configures slider captcha generation and verification.
type CaptchaOptions struct {
	TTLSeconds      int
	SliderTolerance int
	RequireTrack    bool
	UseImagePool    bool
	TrackValidation TrackValidationOptions
}

// TrackValidationOptions controls optional mouse-track validation.
type TrackValidationOptions struct {
	Enabled        bool
	MinPoints      int
	MinDurationMs  int64
	MaxDurationMs  int64
	PointTolerance int
}

// CaptchaUseCase implements slider captcha generation and verification.
type CaptchaUseCase struct {
	opts      CaptchaOptions
	answers   appports.CaptchaAnswerRepository
	generator appports.SliderGenerator
	imagePool appports.BackgroundImagePool
}

var _ appports.CaptchaUseCase = (*CaptchaUseCase)(nil)

func NewCaptchaUseCase(
	answers appports.CaptchaAnswerRepository,
	generator appports.SliderGenerator,
	imagePool appports.BackgroundImagePool,
	opts CaptchaOptions,
) *CaptchaUseCase {
	return &CaptchaUseCase{
		opts:      normalizeCaptchaOptions(opts),
		answers:   answers,
		generator: generator,
		imagePool: imagePool,
	}
}

func (u *CaptchaUseCase) Generate(ctx context.Context) (domain.SliderChallenge, error) {
	if u == nil || u.generator == nil {
		return domain.SliderChallenge{}, fmt.Errorf("captcha generator is not configured")
	}
	if u.answers == nil {
		return domain.SliderChallenge{}, fmt.Errorf("captcha answer repository is not configured")
	}

	var background []byte
	if u.opts.UseImagePool && u.imagePool != nil {
		if imageData, err := u.imagePool.Random(ctx); err == nil {
			background = imageData
		}
	}

	generated, err := u.generator.Generate(ctx, background)
	if err != nil {
		return domain.SliderChallenge{}, err
	}

	captchaID, err := randomHex(16)
	if err != nil {
		return domain.SliderChallenge{}, err
	}

	if err := u.answers.Save(ctx, captchaID, generated.Answer, u.ttl()); err != nil {
		return domain.SliderChallenge{}, err
	}

	return domain.SliderChallenge{
		CaptchaID:         captchaID,
		MasterImage:       generated.MasterImage,
		TileImage:         generated.TileImage,
		TargetY:           generated.TargetY,
		ExpiresIn:         u.opts.TTLSeconds,
		RequireMouseTrack: u.opts.RequireTrack,
	}, nil
}

func (u *CaptchaUseCase) Verify(ctx context.Context, cmd appports.VerifyCaptchaCommand) (appports.VerifyCaptchaResult, error) {
	if cmd.CaptchaID == "" {
		return appports.VerifyCaptchaResult{Valid: false, Reason: "CAPTCHA_ID_EMPTY"}, nil
	}
	if u == nil || u.answers == nil {
		return appports.VerifyCaptchaResult{Valid: false, Reason: "REDIS_ERROR"}, fmt.Errorf("captcha answer repository is not configured")
	}

	answer, err := u.answers.Get(ctx, cmd.CaptchaID)
	if errors.Is(err, domain.ErrCaptchaNotFound) {
		return appports.VerifyCaptchaResult{Valid: false, Reason: "CAPTCHA_NOT_FOUND"}, nil
	}
	if err != nil {
		return appports.VerifyCaptchaResult{Valid: false, Reason: "REDIS_ERROR"}, err
	}

	if !sliderPositionMatches(cmd.PointX, cmd.PointY, answer.DX, answer.DY, u.opts.SliderTolerance) {
		_ = u.answers.Delete(ctx, cmd.CaptchaID)
		return appports.VerifyCaptchaResult{Valid: false, Reason: "CAPTCHA_MISMATCH"}, nil
	}

	if u.opts.TrackValidation.Enabled {
		if !cmd.MouseTrackProvided || len(cmd.MouseTrack) == 0 {
			_ = u.answers.Delete(ctx, cmd.CaptchaID)
			return appports.VerifyCaptchaResult{Valid: false, Reason: "TRACK_REQUIRED"}, nil
		}

		trackResult := validateTrack(u.opts.TrackValidation, cmd.MouseTrack, answer.DX, answer.DY)
		if !trackResult.Valid {
			_ = u.answers.Delete(ctx, cmd.CaptchaID)
			return appports.VerifyCaptchaResult{Valid: false, Reason: trackResult.Code}, nil
		}
	}

	_ = u.answers.Delete(ctx, cmd.CaptchaID)
	return appports.VerifyCaptchaResult{Valid: true, Reason: "OK"}, nil
}

func normalizeCaptchaOptions(opts CaptchaOptions) CaptchaOptions {
	if opts.TTLSeconds <= 0 {
		opts.TTLSeconds = 120
	}
	if opts.SliderTolerance <= 0 {
		opts.SliderTolerance = 8
	}
	return opts
}

func (u *CaptchaUseCase) ttl() time.Duration {
	return time.Duration(u.opts.TTLSeconds) * time.Second
}

func sliderPositionMatches(sx, sy, dx, dy, padding int) bool {
	newX := padding * 2
	newY := padding * 2
	newDx := dx - padding
	newDy := dy - padding

	return sx >= newDx &&
		sx <= newDx+newX &&
		sy >= newDy &&
		sy <= newDy+newY
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
