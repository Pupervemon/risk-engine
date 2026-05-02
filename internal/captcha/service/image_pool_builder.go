package service

import (
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func newConfiguredImagePool(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *RedisImagePool {
	if cfg == nil || !cfg.ImagePool.Enabled {
		return nil
	}

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

	width := normalizedWidth(cfg.Width)
	height := normalizedHeight(cfg.Height)
	providerFactory := NewExternalImageProviderFactory(logger, width, height)
	provider := providerFactory.BuildImagePoolProvider(apiConfig)
	imagePool := NewRedisImagePool(rdb, logger, provider, poolSize)

	if logger != nil {
		logger.Info("图片池已初始化",
			zap.Int("pool_size", poolSize),
			zap.Bool("enabled", true))
	}

	return imagePool
}
