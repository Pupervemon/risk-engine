package bootstrap

import (
	captchaadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/captcha"
	imagepooladapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/imagepool"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func CaptchaOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaapp.CaptchaOptions {
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

func SlideGeneratorOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaadapter.SlideGeneratorOptions {
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

func LifecycleOptionsFromSharedConfig(cfg *config.CaptchaConfigSpec) captchaapp.LifecycleOptions {
	if cfg == nil {
		return captchaapp.LifecycleOptions{RefreshOnStartupProbe: true}
	}

	return captchaapp.LifecycleOptions{
		ImagePoolEnabled:      cfg.ImagePool.Enabled,
		ImageRefreshInterval:  cfg.ImagePool.GetRefreshInterval(),
		RefreshOnStartupProbe: true,
	}
}

func TokenOptionsFromSharedConfig(cfg *config.TokenConfig) captchaapp.TokenOptions {
	if cfg == nil {
		return captchaapp.TokenOptions{}
	}

	return captchaapp.TokenOptions{
		TTLSeconds: cfg.TTLSeconds,
		Secret:     cfg.Secret,
	}
}

func ImageSourceRuntimeConfigFromShared(cfg config.ExternalImageAPIConfig) domain.ImageSourceRuntimeConfig {
	return domain.ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func NewConfiguredImagePool(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *imagepooladapter.RedisImagePool {
	if cfg == nil || !cfg.ImagePool.Enabled {
		return nil
	}

	poolSize := imagePoolSizeFromSharedConfig(cfg)
	imagePool := imagepooladapter.NewRedisImagePool(rdb, logger, poolSize)

	if logger != nil {
		logger.Info("image pool initialized",
			zap.Int("pool_size", poolSize),
			zap.Bool("enabled", true))
	}

	return imagePool
}

func imagePoolSizeFromSharedConfig(cfg *config.CaptchaConfigSpec) int {
	if cfg == nil || cfg.ImagePool.PoolSize <= 0 {
		return 50
	}
	return cfg.ImagePool.PoolSize
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
