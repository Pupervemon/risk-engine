package service

import (
	imagepooladapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/imagepool"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var ErrImagePoolRefreshInProgress = imagepooladapter.ErrImagePoolRefreshInProgress

// ImageMeta contains normalized image data stored in the captcha image pool.
type ImageMeta = domain.ImageMeta

// ImageProvider fetches images from an upstream source.
type ImageProvider = appports.ImageProvider

type RedisImagePool = imagepooladapter.RedisImagePool

// ImagePoolSnapshot describes the current active image-pool generation state.
type ImagePoolSnapshot = domain.ImagePoolSnapshot

// NewRedisImagePool creates a Redis-backed image pool.
func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
	return imagepooladapter.NewRedisImagePool(rdb, logger, provider, poolSize)
}
