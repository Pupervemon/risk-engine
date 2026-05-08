package imagepool

import (
	"context"
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

// ErrImagePoolRefreshInProgress 表示图片池刷新正在进行。
var ErrImagePoolRefreshInProgress = domain.ErrImagePoolRefreshInProgress

// ImageMeta 表示图片池中使用的图片元数据类型。
type ImageMeta = domain.ImageMeta

// ImageProvider 表示图片池使用的图片提供者接口。
type ImageProvider = appports.ImageProvider

// RedisImagePool 基于 Redis 实现验证码图片池。
type RedisImagePool struct {
	logger     *zap.Logger
	provider   ImageProvider
	poolSize   int
	repository imagePoolRepository
	refresher  *imagePoolRefresher
	scheduler  *imagePoolRefreshScheduler
	refreshMu  sync.Mutex
}

// ImagePoolSnapshot 表示图片池当前状态的快照。
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

// NewRedisImagePool 创建一个基于 Redis 的图片池实例。
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

// PoolSize 返回图片池配置的容量。
func (p *RedisImagePool) PoolSize() int {
	if p == nil {
		return 0
	}
	return p.poolSize
}

// HasProvider 判断图片池是否已配置图片提供者。
func (p *RedisImagePool) HasProvider() bool {
	return p != nil && p.provider != nil
}

// SetProvider 设置图片池使用的图片提供者。
func (p *RedisImagePool) SetProvider(provider ImageProvider) {
	if p != nil {
		p.provider = provider
	}
}

// LoadImages 将图片加载到图片池中。
func (p *RedisImagePool) LoadImages(ctx context.Context, images []ImageMeta) error {
	if p == nil || p.refresher == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	return p.refresher.LoadImages(ctx, images)
}

// GetRandom 从图片池中随机获取一张图片。
func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return nil, err
	}
	return repository.Random(ctx)
}

// Count 返回图片池中的图片数量。
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return 0, err
	}
	return repository.Count(ctx)
}

// Snapshot 返回图片池当前内容的快照。
func (p *RedisImagePool) Snapshot(ctx context.Context) (ImagePoolSnapshot, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return ImagePoolSnapshot{}, err
	}

	return repository.Snapshot(ctx)
}

// StartRefresh 启动图片池的后台刷新任务。
func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if p != nil && p.scheduler != nil {
		p.scheduler.Start(ctx, interval, refreshOnStartup)
	}
}

// StopRefresh 停止图片池的后台刷新任务。
func (p *RedisImagePool) StopRefresh() {
	if p != nil && p.scheduler != nil {
		p.scheduler.Stop()
	}
}

// RefreshNow 立即使用当前配置的图片提供者刷新图片池。
func (p *RedisImagePool) RefreshNow(ctx context.Context) error {
	return p.RefreshWithProvider(ctx, p.provider)
}

// RefreshWithProvider 使用指定的图片提供者刷新图片池。
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
