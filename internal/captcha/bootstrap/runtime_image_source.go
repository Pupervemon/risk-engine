package bootstrap

import (
	"context"
	"time"

	imageadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/image"
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const runtimeImageSourceRestoreTimeout = 3 * time.Second

type RuntimeImageSourceComponents struct {
	Manager appports.RuntimeImageSourceManager
	UseCase appports.ImageSourceUseCase
}

func NewRuntimeImageSourceComponents(
	rdb *redis.Client,
	cfg *config.CaptchaConfigSpec,
	imagePool appports.ManagedBackgroundImagePool,
	logger *zap.Logger,
) (RuntimeImageSourceComponents, error) {
	if imagePool == nil {
		return RuntimeImageSourceComponents{
			UseCase: captchaapp.NewImageSourceUseCase(nil, nil, nil, captchaapp.ImageSourceOptions{}),
		}, nil
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

	providerFactory := imageadapter.NewExternalImageProviderFactory(logger, width, height)
	manager, err := captchaapp.NewRuntimeImageSourceManager(
		ImageSourceRuntimeConfigFromShared(externalImageAPI),
		logger,
		width,
		height,
		providerFactory,
	)
	if err != nil {
		return RuntimeImageSourceComponents{}, err
	}

	store := redisadapter.NewImageSourceStore(rdb)
	restoreRuntimeImageSource(store, manager, logger)
	imagePool.SetProvider(manager)

	return RuntimeImageSourceComponents{
		Manager: manager,
		UseCase: captchaapp.NewImageSourceUseCase(
			manager,
			imagePool,
			store,
			captchaapp.ImageSourceOptions{PoolSize: imagePool.PoolSize()},
		),
	}, nil
}

func restoreRuntimeImageSource(store appports.RuntimeImageSourceStore, manager appports.RuntimeImageSourceManager, logger *zap.Logger) {
	if store == nil || manager == nil {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), runtimeImageSourceRestoreTimeout)
	defer cancel()

	persisted, found, err := store.Load(ctx)
	if err != nil {
		logger.Warn("failed to load persisted runtime image source config", zap.Error(err))
		return
	}
	if !found {
		return
	}

	provider, err := manager.BuildProvider(persisted)
	if err != nil {
		logger.Warn("persisted runtime image source config is invalid; keeping file config",
			zap.Error(err),
			zap.String("url", persisted.URL))
		return
	}

	manager.RestoreConfig(persisted, provider)
	logger.Info("restored runtime image source config from redis", zap.String("url", persisted.URL))
}
