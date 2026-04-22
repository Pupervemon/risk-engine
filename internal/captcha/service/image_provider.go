package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	imagePoolRefreshLockTTL            = 15 * time.Minute
	imagePoolRefreshLockAcquireTimeout = 30 * time.Second
	imagePoolRefreshLockRetryInterval  = 500 * time.Millisecond
	imagePoolGenerationsToKeep         = 3
)

var (
	ErrImagePoolRefreshInProgress = errors.New("captcha image pool refresh is already in progress")

	releaseImagePoolRefreshLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)
)

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

// RedisImagePool stores captcha background images in Redis.
type RedisImagePool struct {
	rdb           *redis.Client
	logger        *zap.Logger
	provider      ImageProvider
	poolSize      int
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
		rdb:      rdb,
		logger:   logger,
		provider: provider,
		poolSize: poolSize,
		stopChan: make(chan struct{}),
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
	generation, err := p.activeGeneration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active image generation: %w", err)
	}
	if generation == "" {
		return nil, fmt.Errorf("image pool is empty")
	}

	imageID, err := p.rdb.SRandMember(ctx, p.generationIndexKey(generation)).Result()
	if err == redis.Nil || imageID == "" {
		return nil, fmt.Errorf("image pool is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get image ID: %w", err)
	}

	data, err := p.rdb.Get(ctx, p.generationImageKey(generation, imageID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get image data: %w", err)
	}

	return data, nil
}

// Count returns the current number of images in the active pool generation.
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	generation, err := p.activeGeneration(ctx)
	if err != nil {
		return 0, err
	}
	if generation == "" {
		return 0, nil
	}

	return p.rdb.SCard(ctx, p.generationIndexKey(generation)).Result()
}

// Snapshot returns the currently exposed image-pool generation information.
func (p *RedisImagePool) Snapshot(ctx context.Context) (ImagePoolSnapshot, error) {
	generation, err := p.activeGeneration(ctx)
	if err != nil {
		return ImagePoolSnapshot{}, fmt.Errorf("get active image generation: %w", err)
	}

	snapshot := ImagePoolSnapshot{
		ActiveGeneration: generation,
	}

	pipe := p.rdb.Pipeline()
	generationCountCmd := pipe.ZCard(ctx, p.generationsKey())

	var imageCountCmd *redis.IntCmd
	if generation != "" {
		imageCountCmd = pipe.SCard(ctx, p.generationIndexKey(generation))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return ImagePoolSnapshot{}, fmt.Errorf("inspect image pool snapshot: %w", err)
	}

	snapshot.GenerationCount = generationCountCmd.Val()
	if imageCountCmd != nil {
		snapshot.ImageCount = imageCountCmd.Val()
	}

	return snapshot, nil
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

	previousGeneration, err := p.activeGeneration(ctx)
	if err != nil {
		return fmt.Errorf("get active generation before refresh: %w", err)
	}

	pipe := p.rdb.Pipeline()
	for _, img := range images {
		pipe.Set(ctx, p.generationImageKey(generation, img.ID), img.Data, 0)
		pipe.SAdd(ctx, p.generationIndexKey(generation), img.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		p.logger.Error("failed to load images into redis",
			zap.Error(err),
			zap.String("generation", generation))
		return err
	}

	if err := p.activateGeneration(ctx, generation); err != nil {
		_ = p.deleteGeneration(ctx, generation)
		return fmt.Errorf("activate image generation %s: %w", generation, err)
	}

	if err := p.cleanupStaleGenerations(ctx); err != nil {
		p.logger.Warn("failed to cleanup stale image generations", zap.Error(err))
	}

	p.logger.Info("images loaded into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation),
		zap.String("previous_generation", previousGeneration),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

func (p *RedisImagePool) activateGeneration(ctx context.Context, generation string) error {
	pipe := p.rdb.TxPipeline()
	pipe.Set(ctx, p.activeGenerationKey(), generation, 0)
	pipe.ZAdd(ctx, p.generationsKey(), redis.Z{
		Score:  float64(time.Now().UnixNano()),
		Member: generation,
	})

	_, err := pipe.Exec(ctx)
	return err
}

func (p *RedisImagePool) cleanupStaleGenerations(ctx context.Context) error {
	generations, err := p.rdb.ZRange(ctx, p.generationsKey(), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("list image generations: %w", err)
	}
	if len(generations) <= imagePoolGenerationsToKeep {
		return nil
	}

	staleGenerations := generations[:len(generations)-imagePoolGenerationsToKeep]
	for _, generation := range staleGenerations {
		if generation == "" {
			continue
		}
		if err := p.deleteGeneration(ctx, generation); err != nil {
			return err
		}
		if err := p.rdb.ZRem(ctx, p.generationsKey(), generation).Err(); err != nil {
			return fmt.Errorf("remove stale image generation %s: %w", generation, err)
		}
	}

	return nil
}

func (p *RedisImagePool) deleteGeneration(ctx context.Context, generation string) error {
	ids, err := p.rdb.SMembers(ctx, p.generationIndexKey(generation)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("list image IDs for generation %s: %w", generation, err)
	}

	pipe := p.rdb.Pipeline()
	for _, imageID := range ids {
		pipe.Del(ctx, p.generationImageKey(generation, imageID))
	}
	pipe.Del(ctx, p.generationIndexKey(generation))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete image generation %s: %w", generation, err)
	}

	return nil
}

func (p *RedisImagePool) activeGeneration(ctx context.Context) (string, error) {
	generation, err := p.rdb.Get(ctx, p.activeGenerationKey()).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return generation, nil
}

func (p *RedisImagePool) acquireRefreshLock(ctx context.Context) (string, error) {
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
		ok, err := p.rdb.SetNX(lockCtx, p.refreshLockKey(), token, imagePoolRefreshLockTTL).Result()
		if err != nil {
			if lockCtx.Err() != nil {
				return "", ErrImagePoolRefreshInProgress
			}
			return "", fmt.Errorf("acquire image pool refresh lock: %w", err)
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

	releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := releaseImagePoolRefreshLockScript.Run(releaseCtx, p.rdb, []string{p.refreshLockKey()}, token).Err(); err != nil {
		p.logger.Warn("failed to release image pool refresh lock", zap.Error(err))
	}
}

func (p *RedisImagePool) activeGenerationKey() string {
	return "captcha:images:active_generation"
}

func (p *RedisImagePool) generationsKey() string {
	return "captcha:images:generations"
}

func (p *RedisImagePool) generationImageKey(generation, imageID string) string {
	return fmt.Sprintf("captcha:images:g:%s:data:%s", generation, imageID)
}

func (p *RedisImagePool) generationIndexKey(generation string) string {
	return fmt.Sprintf("captcha:images:g:%s:index", generation)
}

func (p *RedisImagePool) refreshLockKey() string {
	return "captcha:images:refresh:lock"
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
