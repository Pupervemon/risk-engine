package bootstrap

import (
	captchaadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/captcha"
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type CaptchaComponents struct {
	ImagePool appports.BackgroundImagePool
	Captcha   appports.CaptchaUseCase
}

func NewCaptchaComponents(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) CaptchaComponents {
	if cfg == nil {
		cfg = &config.CaptchaConfigSpec{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	imagePool := NewConfiguredImagePool(rdb, cfg, logger)
	var imagePoolPort appports.BackgroundImagePool
	if imagePool != nil {
		imagePoolPort = imagePool
	}

	return CaptchaComponents{
		ImagePool: imagePoolPort,
		Captcha: captchaapp.NewCaptchaUseCase(
			redisadapter.NewCaptchaAnswerRepository(rdb),
			captchaadapter.NewSlideGenerator(SlideGeneratorOptionsFromSharedConfig(cfg), logger),
			imagePoolPort,
			CaptchaOptionsFromSharedConfig(cfg),
		),
	}
}

func NewCaptchaLifecycle(
	imagePool appports.BackgroundImagePool,
	imageSource appports.ImageSourceUseCase,
	cfg *config.CaptchaConfigSpec,
	logger *zap.Logger,
) appports.CaptchaLifecycle {
	return captchaapp.NewCaptchaLifecycle(
		imagePool,
		imageSource,
		LifecycleOptionsFromSharedConfig(cfg),
		logger,
	)
}
