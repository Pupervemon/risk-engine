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

// CaptchaOptionsFromSharedConfig 将共享配置转换为验证码用例所需的运行参数。
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

// SlideGeneratorOptionsFromSharedConfig 将共享配置转换为滑块生成器参数。
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

// LifecycleOptionsFromSharedConfig 生成图片池生命周期相关的配置。
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

// TokenOptionsFromSharedConfig 将 token 相关配置转换为用例参数。
func TokenOptionsFromSharedConfig(cfg *config.TokenConfig) captchaapp.TokenOptions {
	if cfg == nil {
		return captchaapp.TokenOptions{}
	}

	return captchaapp.TokenOptions{
		TTLSeconds: cfg.TTLSeconds,
		Secret:     cfg.Secret,
	}
}

// ImageSourceRuntimeConfigFromShared 将外部图片源配置转换为领域层运行时配置。
func ImageSourceRuntimeConfigFromShared(cfg config.ExternalImageAPIConfig) domain.ImageSourceRuntimeConfig {
	return domain.ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

// NewConfiguredImagePool 根据共享配置构造 Redis 图片池，并在启用时注入外部图片提供者。
func NewConfiguredImagePool(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *imagepooladapter.RedisImagePool {
	if cfg == nil || !cfg.ImagePool.Enabled {
		return nil
	}

	// 池大小缺省时使用一个保守的默认值，避免空配置导致无法启动。
	poolSize := cfg.ImagePool.PoolSize
	if poolSize <= 0 {
		poolSize = 50
	}

	imagePool := imagepooladapter.NewRedisImagePool(rdb, logger, nil, poolSize)

	if logger != nil {
		logger.Info("image pool initialized",
			zap.Int("pool_size", poolSize),
			zap.Bool("enabled", true))
	}

	return imagePool
}

// normalizedWidth 将非法或缺省的宽度回退到默认值。
func normalizedWidth(width int) int {
	if width <= 0 {
		return 320
	}
	return width
}

// normalizedHeight 将非法或缺省的高度回退到默认值。
func normalizedHeight(height int) int {
	if height <= 0 {
		return 180
	}
	return height
}
