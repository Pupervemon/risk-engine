package service

import (
	"context"
	"fmt"
	"sync"

	captchaadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/captcha"
	imagepooladapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/imagepool"
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type CaptchaService struct {
	rdb                *redis.Client
	cfg                *config.CaptchaConfigSpec
	logger             *zap.Logger
	imagePool          *imagepooladapter.RedisImagePool
	useImagePool       bool
	captchaUseCase     appports.CaptchaUseCase
	lifecycle          appports.CaptchaLifecycle
	imageSourceUseCase appports.ImageSourceUseCase
	imageSourceMu      sync.RWMutex
	imageSourceBinding *runtimeImageSourceBinding
}

// SliderChallenge is the legacy service DTO returned to older callers.
type SliderChallenge struct {
	CaptchaID         string
	MasterImage       string
	TileImage         string
	TargetY           int
	ExpiresIn         int
	RequireMouseTrack bool
}

// NewCaptchaService initializes the legacy captcha service wrapper.
func NewCaptchaService(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *CaptchaService {
	if cfg == nil {
		cfg = &config.CaptchaConfigSpec{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	service := &CaptchaService{
		rdb:          rdb,
		cfg:          cfg,
		logger:       logger,
		useImagePool: cfg.ImagePool.Enabled,
	}

	service.imagePool = newConfiguredImagePool(rdb, cfg, logger)

	if cfg.TrackValidation.Enabled {
		logger.Info("轨迹校验已启用",
			zap.Int("min_points", cfg.TrackValidation.MinPoints),
			zap.Int64("min_duration_ms", cfg.TrackValidation.MinDurationMs))
	}

	var imagePool appports.BackgroundImagePool
	if service.imagePool != nil {
		imagePool = imagepooladapter.NewPortAdapter(service.imagePool)
	}

	service.captchaUseCase = captchaapp.NewCaptchaUseCase(
		redisadapter.NewCaptchaAnswerRepository(rdb),
		captchaadapter.NewSlideGenerator(slideGeneratorOptionsFromSharedConfig(cfg), logger),
		imagePool,
		captchaOptionsFromSharedConfig(cfg),
	)
	service.lifecycle = captchaapp.NewCaptchaLifecycle(
		imagePool,
		lifecycleOptionsFromSharedConfig(cfg),
		logger,
	)

	return service
}

// Generate delegates slider captcha generation to the application use case.
func (s *CaptchaService) Generate(ctx context.Context) (*SliderChallenge, error) {
	if s == nil || s.captchaUseCase == nil {
		return nil, fmt.Errorf("captcha use case is not configured")
	}

	challenge, err := s.captchaUseCase.Generate(ctx)
	if err != nil {
		return nil, err
	}

	return &SliderChallenge{
		CaptchaID:         challenge.CaptchaID,
		MasterImage:       challenge.MasterImage,
		TileImage:         challenge.TileImage,
		TargetY:           challenge.TargetY,
		ExpiresIn:         challenge.ExpiresIn,
		RequireMouseTrack: challenge.RequireMouseTrack,
	}, nil
}

// Verify verifies a slider captcha without mouse-track data.
func (s *CaptchaService) Verify(ctx context.Context, captchaID string, pointX, pointY int) (bool, string, error) {
	return s.VerifyWithTrack(ctx, captchaID, pointX, pointY, nil)
}

// VerifyWithTrack verifies a slider captcha with optional mouse-track data.
func (s *CaptchaService) VerifyWithTrack(ctx context.Context, captchaID string, pointX, pointY int, mouseTrack *[]TrackPoint) (bool, string, error) {
	if s == nil || s.captchaUseCase == nil {
		return false, "REDIS_ERROR", fmt.Errorf("captcha use case is not configured")
	}

	result, err := s.captchaUseCase.Verify(ctx, appports.VerifyCaptchaCommand{
		CaptchaID:          captchaID,
		PointX:             pointX,
		PointY:             pointY,
		MouseTrack:         serviceTrackPointsToDomain(mouseTrack),
		MouseTrackProvided: mouseTrack != nil,
	})
	return result.Valid, result.Reason, err
}

// StartImageRefresh starts the image-pool refresh job.
func (s *CaptchaService) StartImageRefresh(ctx context.Context) error {
	if s == nil || s.lifecycle == nil {
		return nil
	}
	return s.lifecycle.StartImageRefresh(ctx)
}

// StopImageRefresh stops the image-pool refresh job.
func (s *CaptchaService) StopImageRefresh() {
	if s != nil && s.lifecycle != nil {
		s.lifecycle.StopImageRefresh()
	}
}

// GetImagePoolStatus returns image-pool status for health checks.
func (s *CaptchaService) GetImagePoolStatus(ctx context.Context) map[string]interface{} {
	status := map[string]interface{}{
		"enabled": s.useImagePool,
	}

	if s.useImagePool && s.imagePool != nil {
		count, err := s.imagePool.Count(ctx)
		if err != nil {
			status["error"] = err.Error()
			status["count"] = 0
		} else {
			status["count"] = count
		}
	}

	return status
}

func captchaOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaapp.CaptchaOptions {
	if cfg == nil {
		return captchaapp.CaptchaOptions{}
	}

	return captchaapp.CaptchaOptions{
		TTLSeconds:      cfg.TTLSeconds,
		SliderTolerance: cfg.SliderTolerance,
		RequireTrack:    cfg.TrackValidation.Enabled,
		UseImagePool:    cfg.ImagePool.Enabled,
		TrackValidation: captchaapp.TrackValidationOptions{
			Enabled:        cfg.TrackValidation.Enabled,
			MinPoints:      cfg.TrackValidation.MinPoints,
			MinDurationMs:  cfg.TrackValidation.MinDurationMs,
			MaxDurationMs:  cfg.TrackValidation.MaxDurationMs,
			PointTolerance: cfg.TrackValidation.PointTolerance,
		},
	}
}

func slideGeneratorOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaadapter.SlideGeneratorOptions {
	if cfg == nil {
		return captchaadapter.SlideGeneratorOptions{}
	}

	return captchaadapter.SlideGeneratorOptions{
		Width:        cfg.Width,
		Height:       cfg.Height,
		GraphSizeMin: cfg.GraphSizeMin,
		GraphSizeMax: cfg.GraphSizeMax,
	}
}

func lifecycleOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaapp.LifecycleOptions {
	if cfg == nil {
		return captchaapp.LifecycleOptions{RefreshOnStartupProbe: true}
	}

	return captchaapp.LifecycleOptions{
		ImagePoolEnabled:      cfg.ImagePool.Enabled,
		ImageRefreshInterval:  cfg.ImagePool.GetRefreshInterval(),
		RefreshOnStartupProbe: true,
	}
}

func serviceTrackPointsToDomain(points *[]TrackPoint) []domain.TrackPoint {
	if points == nil {
		return nil
	}

	converted := make([]domain.TrackPoint, 0, len(*points))
	for _, point := range *points {
		converted = append(converted, domain.TrackPoint{
			X:    point.X,
			Y:    point.Y,
			Time: point.Time,
		})
	}

	return converted
}

func normalizedWidth(width int) int {
	if width <= 0 {
		return 320
	}
	return width
}

func normalizedHeight(height int) int {
	if height <= 0 {
		return 180
	}
	return height
}
