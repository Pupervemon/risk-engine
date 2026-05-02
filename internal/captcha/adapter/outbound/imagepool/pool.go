package imagepool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	imagePoolRefreshLockTTL            = 15 * time.Minute
	imagePoolRefreshLockAcquireTimeout = 30 * time.Second
	imagePoolRefreshLockRetryInterval  = 500 * time.Millisecond
	imagePoolGenerationsToKeep         = 3
)

var ErrImagePoolRefreshInProgress = errors.New("captcha image pool refresh is already in progress")

type ImageMeta = domain.ImageMeta

type ImageProvider = appports.ImageProvider

type RedisImagePool struct {
	logger     *zap.Logger
	provider   ImageProvider
	poolSize   int
	repository imagePoolRepository
	refresher  *imagePoolRefresher
	scheduler  *imagePoolRefreshScheduler
	refreshMu  sync.Mutex
}

type ImagePoolSnapshot = domain.ImagePoolSnapshot

type imagePoolRepository interface {
	Random(ctx context.Context) ([]byte, error)
	Count(ctx context.Context) (int64, error)
	Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error)
	LoadImagesIntoGeneration(ctx context.Context, generation string, images []domain.ImageMeta) (string, error)
	CleanupStaleGenerations(ctx context.Context, generationsToKeep int) error
	AcquireRefreshLock(ctx context.Context, token string, ttl time.Duration) (bool, error)
	ReleaseRefreshLock(ctx context.Context, token string) error
}

func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
	return newRedisImagePool(redisadapter.NewImagePoolRepository(rdb), logger, provider, poolSize)
}

func newRedisImagePool(repository imagePoolRepository, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
	if logger == nil {
		logger = zap.NewNop()
	}

	pool := &RedisImagePool{
		logger:     logger,
		provider:   provider,
		poolSize:   poolSize,
		repository: repository,
	}
	pool.refresher = newImagePoolRefresher(repository, logger, poolSize)
	pool.scheduler = newImagePoolRefreshScheduler(logger, poolSize, pool.RefreshNow)
	return pool
}

func (p *RedisImagePool) PoolSize() int {
	if p == nil {
		return 0
	}
	return p.poolSize
}

func (p *RedisImagePool) HasProvider() bool {
	return p != nil && p.provider != nil
}

func (p *RedisImagePool) SetProvider(provider ImageProvider) {
	if p != nil {
		p.provider = provider
	}
}

func (p *RedisImagePool) LoadImages(ctx context.Context, images []ImageMeta) error {
	if p == nil || p.refresher == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	return p.refresher.LoadImages(ctx, images)
}

func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return nil, err
	}
	return repository.Random(ctx)
}

func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return 0, err
	}
	return repository.Count(ctx)
}

func (p *RedisImagePool) Snapshot(ctx context.Context) (ImagePoolSnapshot, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return ImagePoolSnapshot{}, err
	}

	return repository.Snapshot(ctx)
}

func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if p != nil && p.scheduler != nil {
		p.scheduler.Start(ctx, interval, refreshOnStartup)
	}
}

func (p *RedisImagePool) StopRefresh() {
	if p != nil && p.scheduler != nil {
		p.scheduler.Stop()
	}
}

func (p *RedisImagePool) RefreshNow(ctx context.Context) error {
	return p.RefreshWithProvider(ctx, p.provider)
}

func (p *RedisImagePool) RefreshWithProvider(ctx context.Context, provider ImageProvider) error {
	if provider == nil {
		return fmt.Errorf("image provider is not configured")
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	return p.refreshWithProvider(ctx, provider)
}

func (p *RedisImagePool) refreshWithProvider(ctx context.Context, provider ImageProvider) error {
	if p == nil || p.refresher == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	return p.refresher.Refresh(ctx, provider)
}

func (p *RedisImagePool) imagePoolRepository() (imagePoolRepository, error) {
	if p == nil || p.repository == nil {
		return nil, fmt.Errorf("image pool repository is not configured")
	}
	return p.repository, nil
}
