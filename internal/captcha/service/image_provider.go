package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// ImageMeta contains normalized image data stored in the captcha image pool.
type ImageMeta = domain.ImageMeta

// ImageProvider fetches images from an upstream source.
type ImageProvider = appports.ImageProvider

// RedisImagePool coordinates image-pool refresh scheduling while delegating
// Redis generation storage to the outbound repository.
type RedisImagePool struct {
	logger     *zap.Logger
	provider   ImageProvider
	poolSize   int
	repository imagePoolRepository
	scheduler  *imagePoolRefreshScheduler
	refreshMu  sync.Mutex
}

// ImagePoolSnapshot describes the current active image-pool generation state.
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

// NewRedisImagePool creates a Redis-backed image pool.
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
	pool.scheduler = newImagePoolRefreshScheduler(logger, poolSize, pool.RefreshNow)
	return pool
}

// LoadImages replaces the current image pool contents with the supplied images.
func (p *RedisImagePool) LoadImages(ctx context.Context, images []ImageMeta) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to load")
	}

	generation, err := newImagePoolGeneration()
	if err != nil {
		return fmt.Errorf("generate image pool generation: %w", err)
	}

	return p.loadImagesIntoGeneration(ctx, generation, images)
}

// GetRandom returns a random image from the active pool generation.
func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return nil, err
	}
	return repository.Random(ctx)
}

// Count returns the current number of images in the active pool generation.
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return 0, err
	}
	return repository.Count(ctx)
}

// Snapshot returns the currently exposed image-pool generation information.
func (p *RedisImagePool) Snapshot(ctx context.Context) (ImagePoolSnapshot, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return ImagePoolSnapshot{}, err
	}

	return repository.Snapshot(ctx)
}

// StartRefresh starts the image-pool refresh job.
// It may perform an immediate warm-up refresh and then keeps the pool fresh via
// the configured interval plus a local-midnight daily refresh.
func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if p != nil && p.scheduler != nil {
		p.scheduler.Start(ctx, interval, refreshOnStartup)
	}
}

// StopRefresh stops the periodic image refresh job.
func (p *RedisImagePool) StopRefresh() {
	if p != nil && p.scheduler != nil {
		p.scheduler.Stop()
	}
}

// RefreshNow refreshes the pool using the active provider.
func (p *RedisImagePool) RefreshNow(ctx context.Context) error {
	return p.RefreshWithProvider(ctx, p.provider)
}

// RefreshWithProvider refreshes the pool using a supplied provider without replacing the active one.
func (p *RedisImagePool) RefreshWithProvider(ctx context.Context, provider ImageProvider) error {
	if provider == nil {
		return fmt.Errorf("image provider is not configured")
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	return p.refreshWithProvider(ctx, provider)
}

func (p *RedisImagePool) refreshWithProvider(ctx context.Context, provider ImageProvider) error {
	lockToken, err := p.acquireRefreshLock(ctx)
	if err != nil {
		return err
	}
	defer p.releaseRefreshLock(lockToken)

	p.logger.Info("refreshing image pool", zap.Int("target_size", p.poolSize))
	startTime := time.Now()

	oldCount, _ := p.Count(ctx)

	images, err := provider.FetchImages(ctx, p.poolSize)
	if err != nil {
		return fmt.Errorf("failed to fetch images: %w", err)
	}

	if err := p.LoadImages(ctx, images); err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	newCount, _ := p.Count(ctx)

	p.logger.Info("image pool refreshed",
		zap.Int64("old_count", oldCount),
		zap.Int64("new_count", newCount),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

func (p *RedisImagePool) loadImagesIntoGeneration(ctx context.Context, generation string, images []ImageMeta) error {
	if generation == "" {
		return fmt.Errorf("image pool generation is required")
	}

	p.logger.Info("loading images into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation))
	startTime := time.Now()

	repository, err := p.imagePoolRepository()
	if err != nil {
		return err
	}

	previousGeneration, err := repository.LoadImagesIntoGeneration(ctx, generation, images)
	if err != nil {
		p.logger.Error("failed to load images into redis",
			zap.Error(err),
			zap.String("generation", generation))
		return err
	}

	if err := repository.CleanupStaleGenerations(ctx, imagePoolGenerationsToKeep); err != nil {
		p.logger.Warn("failed to cleanup stale image generations", zap.Error(err))
	}

	p.logger.Info("images loaded into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation),
		zap.String("previous_generation", previousGeneration),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

func (p *RedisImagePool) acquireRefreshLock(ctx context.Context) (string, error) {
	repository, err := p.imagePoolRepository()
	if err != nil {
		return "", err
	}

	lockCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		lockCtx, cancel = context.WithTimeout(ctx, imagePoolRefreshLockAcquireTimeout)
	}
	defer cancel()

	token, err := newImagePoolToken()
	if err != nil {
		return "", fmt.Errorf("generate refresh lock token: %w", err)
	}

	retryTicker := time.NewTicker(imagePoolRefreshLockRetryInterval)
	defer retryTicker.Stop()

	for {
		ok, err := repository.AcquireRefreshLock(lockCtx, token, imagePoolRefreshLockTTL)
		if err != nil {
			if lockCtx.Err() != nil {
				return "", ErrImagePoolRefreshInProgress
			}
			return "", err
		}
		if ok {
			return token, nil
		}

		select {
		case <-lockCtx.Done():
			return "", ErrImagePoolRefreshInProgress
		case <-retryTicker.C:
		}
	}
}

func (p *RedisImagePool) releaseRefreshLock(token string) {
	if token == "" {
		return
	}

	repository, err := p.imagePoolRepository()
	if err != nil {
		p.logger.Warn("failed to release image pool refresh lock", zap.Error(err))
		return
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := repository.ReleaseRefreshLock(releaseCtx, token); err != nil {
		p.logger.Warn("failed to release image pool refresh lock", zap.Error(err))
	}
}

func newImagePoolGeneration() (string, error) {
	suffix, err := newImagePoolToken()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), suffix), nil
}

func newImagePoolToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

func (p *RedisImagePool) imagePoolRepository() (imagePoolRepository, error) {
	if p == nil || p.repository == nil {
		return nil, fmt.Errorf("image pool repository is not configured")
	}
	return p.repository, nil
}
