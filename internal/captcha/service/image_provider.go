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
type ImageMeta struct {
	ID   string
	Data []byte
	URL  string
}

// ImageProvider fetches images from an upstream source.
type ImageProvider interface {
	FetchImages(ctx context.Context, count int) ([]ImageMeta, error)
}

// RedisImagePool coordinates image-pool refresh scheduling while delegating
// Redis generation storage to the outbound repository.
type RedisImagePool struct {
	logger        *zap.Logger
	provider      ImageProvider
	poolSize      int
	repository    *redisadapter.ImagePoolRepository
	refreshTicker *time.Ticker
	midnightTimer *time.Timer
	stopChan      chan struct{}
	stopOnce      sync.Once
	refreshMu     sync.Mutex
}

// ImagePoolSnapshot describes the current active image-pool generation state.
type ImagePoolSnapshot struct {
	ImageCount       int64
	ActiveGeneration string
	GenerationCount  int64
}

// NewRedisImagePool creates a Redis-backed image pool.
func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RedisImagePool{
		logger:     logger,
		provider:   provider,
		poolSize:   poolSize,
		repository: redisadapter.NewImagePoolRepository(rdb),
		stopChan:   make(chan struct{}),
	}
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

	snapshot, err := repository.Snapshot(ctx)
	if err != nil {
		return ImagePoolSnapshot{}, err
	}

	return ImagePoolSnapshot{
		ImageCount:       snapshot.ImageCount,
		ActiveGeneration: snapshot.ActiveGeneration,
		GenerationCount:  snapshot.GenerationCount,
	}, nil
}

// StartRefresh starts the image-pool refresh job.
// It may perform an immediate warm-up refresh and then keeps the pool fresh via
// the configured interval plus a local-midnight daily refresh.
func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	nextMidnightAt := nextMidnightRefreshTime(time.Now())
	p.logger.Info("starting image pool refresh job",
		zap.Duration("interval", interval),
		zap.Int("pool_size", p.poolSize),
		zap.Bool("refresh_on_startup", refreshOnStartup),
		zap.Time("next_midnight_refresh_at", nextMidnightAt))

	if refreshOnStartup {
		if err := p.RefreshNow(ctx); err != nil {
			p.logger.Error("initial image refresh failed", zap.Error(err))
		}
	} else {
		p.logger.Info("skipping initial image refresh because the pool already has cached images")
	}

	if interval > 0 {
		p.refreshTicker = time.NewTicker(interval)
	}
	p.midnightTimer = time.NewTimer(nextMidnightRefreshDelay(time.Now()))

	go func() {
		for {
			var intervalChan <-chan time.Time
			if p.refreshTicker != nil {
				intervalChan = p.refreshTicker.C
			}

			var midnightChan <-chan time.Time
			if p.midnightTimer != nil {
				midnightChan = p.midnightTimer.C
			}

			select {
			case <-intervalChan:
				if err := p.RefreshNow(ctx); err != nil {
					p.logger.Error("scheduled image refresh failed", zap.Error(err))
				}
			case <-midnightChan:
				if err := p.RefreshNow(ctx); err != nil {
					p.logger.Error("midnight image refresh failed", zap.Error(err))
				}

				nextMidnightAt := nextMidnightRefreshTime(time.Now())
				p.logger.Info("scheduled next midnight image refresh",
					zap.Time("next_midnight_refresh_at", nextMidnightAt))
				p.midnightTimer.Reset(nextMidnightRefreshDelay(time.Now()))
			case <-p.stopChan:
				p.logger.Info("image pool refresh job stopped")
				return
			}
		}
	}()
}

// StopRefresh stops the periodic image refresh job.
func (p *RedisImagePool) StopRefresh() {
	if p.refreshTicker != nil {
		p.refreshTicker.Stop()
	}
	if p.midnightTimer != nil {
		p.midnightTimer.Stop()
	}
	p.stopOnce.Do(func() {
		close(p.stopChan)
	})
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

	previousGeneration, err := repository.LoadImagesIntoGeneration(ctx, generation, imageMetasToDomain(images))
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

func nextMidnightRefreshTime(now time.Time) time.Time {
	return time.Date(
		now.Year(),
		now.Month(),
		now.Day()+1,
		0, 0, 0, 0,
		now.Location(),
	)
}

func nextMidnightRefreshDelay(now time.Time) time.Duration {
	return nextMidnightRefreshTime(now).Sub(now)
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

func (p *RedisImagePool) imagePoolRepository() (*redisadapter.ImagePoolRepository, error) {
	if p == nil || p.repository == nil {
		return nil, fmt.Errorf("image pool repository is not configured")
	}
	return p.repository, nil
}

func imageMetasToDomain(images []ImageMeta) []domain.ImageMeta {
	converted := make([]domain.ImageMeta, 0, len(images))
	for _, image := range images {
		converted = append(converted, domain.ImageMeta{
			ID:   image.ID,
			Data: image.Data,
			URL:  image.URL,
		})
	}
	return converted
}
