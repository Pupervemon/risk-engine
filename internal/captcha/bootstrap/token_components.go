package bootstrap

import (
	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
)

func NewTokenUseCase(rdb *redis.Client, cfg *config.TokenConfig) appports.TokenUseCase {
	return captchaapp.NewTokenUseCase(
		redisadapter.NewTokenRepository(rdb),
		TokenOptionsFromSharedConfig(cfg),
	)
}

func NewTokenUseCaseWithRepository(repo appports.TokenRepository, cfg *config.TokenConfig) appports.TokenUseCase {
	return captchaapp.NewTokenUseCase(repo, TokenOptionsFromSharedConfig(cfg))
}
