package service

import (
	"context"
	"fmt"

	captchaadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/captcha"
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type CaptchaService struct {
	rdb            *redis.Client
	cfg            *config.CaptchaConfigSpec
	logger         *zap.Logger
	imagePool      *RedisImagePool
	useImagePool   bool
	captchaUseCase appports.CaptchaUseCase
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

	width := normalizedWidth(cfg.Width)
	height := normalizedHeight(cfg.Height)

	service := &CaptchaService{
		rdb:          rdb,
		cfg:          cfg,
		logger:       logger,
		useImagePool: cfg.ImagePool.Enabled,
	}

	if cfg.ImagePool.Enabled {
		poolSize := cfg.ImagePool.PoolSize
		if poolSize <= 0 {
			poolSize = 50
		}

		apiConfig := ExternalImageAPIConfig{
			URL:                cfg.ExternalImageAPI.URL,
			APIKey:             cfg.ExternalImageAPI.APIKey,
			Timeout:            cfg.ExternalImageAPI.GetTimeout(),
			RateLimitPerMinute: cfg.ExternalImageAPI.RateLimitPerMinute,
			RetryCount:         cfg.ExternalImageAPI.RetryCount,
		}

		providerFactory := NewExternalImageProviderFactory(logger, width, height)
		provider := providerFactory.BuildImagePoolProvider(apiConfig)
		service.imagePool = NewRedisImagePool(rdb, logger, provider, poolSize)

		logger.Info("图片池已初始化",
			zap.Int("pool_size", poolSize),
			zap.Bool("enabled", true))
	}

	if cfg.TrackValidation.Enabled {
		logger.Info("轨迹校验已启用",
			zap.Int("min_points", cfg.TrackValidation.MinPoints),
			zap.Int64("min_duration_ms", cfg.TrackValidation.MinDurationMs))
	}

	var imagePool appports.BackgroundImagePool
	if service.imagePool != nil {
		imagePool = NewImagePoolPortAdapter(service.imagePool)
	}

	service.captchaUseCase = captchaapp.NewCaptchaUseCase(
		redisadapter.NewCaptchaAnswerRepository(rdb),
		captchaadapter.NewSlideGenerator(slideGeneratorOptionsFromSharedConfig(cfg), logger),
		imagePool,
		captchaOptionsFromSharedConfig(cfg),
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
	if !s.useImagePool || s.imagePool == nil {
		s.logger.Info("图片池未启用，跳过刷新任务")
		return nil
	}

	refreshInterval := s.cfg.ImagePool.GetRefreshInterval()
	currentCount, err := s.imagePool.Count(ctx)
	refreshOnStartup := true
	if err != nil {
		s.logger.Warn("failed to inspect image pool before startup refresh; falling back to immediate refresh",
			zap.Error(err))
	} else if !shouldRefreshImagePoolOnStartup(currentCount) {
		refreshOnStartup = false
		s.logger.Info("image pool already contains cached images; skipping immediate startup refresh",
			zap.Int64("current_count", currentCount))
	} else {
		s.logger.Info("image pool is empty; performing immediate startup refresh",
			zap.Int64("current_count", currentCount))
	}

	s.imagePool.StartRefresh(ctx, refreshInterval, refreshOnStartup)

	s.logger.Info("图片池刷新任务已启动",
		zap.Duration("interval", refreshInterval))

	return nil
}

// StopImageRefresh stops the image-pool refresh job.
func (s *CaptchaService) StopImageRefresh() {
	if s.imagePool != nil {
		s.imagePool.StopRefresh()
		s.logger.Info("图片池刷新任务已停止")
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

func shouldRefreshImagePoolOnStartup(existingCount int64) bool {
	return existingCount <= 0
}
