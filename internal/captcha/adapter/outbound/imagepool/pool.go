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

var ErrImagePoolRefreshInProgress = domain.ErrImagePoolRefreshInProgress

type ImageMeta = domain.ImageMeta
type ImageProvider = appports.ImageProvider
type ImagePoolSnapshot = domain.ImagePoolSnapshot

// ImagePool owns captcha background-pool behavior. The repository decides
// where the pool is stored.
type ImagePool struct {
	logger     *zap.Logger
	poolSize   int
	repository imagePoolRepository
	refresher  *imagePoolRefresher
	refreshMu  sync.Mutex
}

var _ appports.BackgroundImagePool = (*ImagePool)(nil)

type imagePoolRepository interface {
	Random(ctx context.Context) ([]byte, error)
	Count(ctx context.Context) (int64, error)
	Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error)
	LoadImagesIntoGeneration(ctx context.Context, generation string, images []domain.ImageMeta, meta domain.ImagePoolGenerationMeta) (string, error)
	CleanupStaleGenerations(ctx context.Context, generationsToKeep int) error
	AcquireRefreshLock(ctx context.Context, token string, ttl time.Duration) (bool, error)
	ReleaseRefreshLock(ctx context.Context, token string) error
}

func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, poolSize int) *ImagePool {
	return newImagePool(redisadapter.NewImagePoolRepository(rdb), logger, poolSize)
}

func newImagePool(repository imagePoolRepository, logger *zap.Logger, poolSize int) *ImagePool {
	if logger == nil {
		logger = zap.NewNop()
	}

	pool := &ImagePool{
		logger:     logger,
		poolSize:   poolSize,
		repository: repository,
	}
	pool.refresher = newImagePoolRefresher(repository, logger, poolSize)
	return pool
}

func (p *ImagePool) PoolSize() int {
	if p == nil {
		return 0
	}
	return p.poolSize
}

func (p *ImagePool) Random(ctx context.Context) ([]byte, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return nil, err
	}
	return repository.Random(ctx)
}

func (p *ImagePool) Count(ctx context.Context) (int64, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return 0, err
	}
	return repository.Count(ctx)
}

func (p *ImagePool) Snapshot(ctx context.Context) (ImagePoolSnapshot, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return ImagePoolSnapshot{}, err
	}

	return repository.Snapshot(ctx)
}

func (p *ImagePool) RefreshWithProvider(ctx context.Context, provider ImageProvider, meta domain.ImagePoolGenerationMeta) error {
	if provider == nil {
		return fmt.Errorf("image provider is not configured")
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	return p.refreshWithProvider(ctx, provider, meta)
}

func (p *ImagePool) refreshWithProvider(ctx context.Context, provider ImageProvider, meta domain.ImagePoolGenerationMeta) error {
	if p == nil || p.refresher == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	return p.refresher.Refresh(ctx, provider, meta)
}

func (p *ImagePool) imagePoolRepository() (imagePoolRepository, error) {
	if p == nil || p.repository == nil {
		return nil, fmt.Errorf("image pool repository is not configured")
	}
	return p.repository, nil
}
