package bootstrap

import (
	"context"
	"fmt"
	"time"

	imageadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/image"
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const runtimeImageSourceInitTimeout = 3 * time.Second

func NewRuntimeImageSourceUseCase(
	rdb *redis.Client,
	cfg *config.CaptchaConfigSpec,
	imagePool appports.BackgroundImagePool,
	logger *zap.Logger,
) (appports.ImageSourceUseCase, error) {
	if imagePool == nil {
		return captchaapp.NewImageSourceUseCase(nil, nil, nil, captchaapp.ImageSourceOptions{}), nil
	}

	width := 320
	height := 180
	if cfg != nil {
		width = normalizedWidth(cfg.Width)
		height = normalizedHeight(cfg.Height)
	}

	var externalImageAPI config.ExternalImageAPIConfig
	if cfg != nil {
		externalImageAPI = cfg.ExternalImageAPI
	}
	poolSize := 50
	if cfg != nil && cfg.ImagePool.PoolSize > 0 {
		poolSize = cfg.ImagePool.PoolSize
	}

	store := redisadapter.NewImageSourceStore(rdb)
	providerFactory := imageadapter.NewExternalImageProviderFactory(logger, width, height)
	if err := ensureRuntimeImageSourceConfig(store, externalImageAPI, logger); err != nil {
		return nil, err
	}

	return captchaapp.NewImageSourceUseCase(
		imagePool,
		store,
		providerFactory,
		captchaapp.ImageSourceOptions{PoolSize: poolSize},
	), nil
}

func ensureRuntimeImageSourceConfig(store appports.RuntimeImageSourceStore, cfg config.ExternalImageAPIConfig, logger *zap.Logger) error {
	if store == nil {
		return fmt.Errorf("runtime image source store is not configured")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), runtimeImageSourceInitTimeout)
	defer cancel()

	persisted, found, err := store.Load(ctx)
	if err != nil {
		return err
	}
	if found {
		if err := persisted.Validate(); err != nil {
			return err
		}
		logger.Info("loaded runtime image source config from redis", zap.String("url", persisted.URL))
		return nil
	}

	initial := ImageSourceRuntimeConfigFromShared(cfg).Normalized()
	initial.Version = 1
	initial.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := initial.Validate(); err != nil {
		return err
	}
	if err := store.Save(ctx, initial); err != nil {
		return err
	}

	logger.Info("initialized runtime image source config in redis", zap.String("url", initial.URL))
	return nil
}
