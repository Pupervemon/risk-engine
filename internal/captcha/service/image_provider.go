package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
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
	stopChan      chan struct{}
	refreshMu     sync.Mutex
}

// NewRedisImagePool creates a Redis-backed image pool.
func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
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

	p.logger.Info("loading images into redis", zap.Int("count", len(images)))
	startTime := time.Now()

	pipe := p.rdb.Pipeline()

	oldIDs, err := p.rdb.SMembers(ctx, p.indexKey()).Result()
	if err == nil && len(oldIDs) > 0 {
		for _, oldID := range oldIDs {
			pipe.Del(ctx, p.imageKey(oldID))
		}
		pipe.Del(ctx, p.indexKey())
	}

	for _, img := range images {
		pipe.Set(ctx, p.imageKey(img.ID), img.Data, 0)
		pipe.SAdd(ctx, p.indexKey(), img.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		p.logger.Error("failed to load images into redis", zap.Error(err))
		return err
	}

	p.logger.Info("images loaded into redis",
		zap.Int("count", len(images)),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// GetRandom returns a random image from the pool.
func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	ids, err := p.rdb.SMembers(ctx, p.indexKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get image IDs: %w", err)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("image pool is empty")
	}

	randomIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(ids))))
	if err != nil {
		return nil, fmt.Errorf("failed to generate random index: %w", err)
	}

	selectedID := ids[randomIdx.Int64()]
	data, err := p.rdb.Get(ctx, p.imageKey(selectedID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get image data: %w", err)
	}

	return data, nil
}

// Count returns the current number of images in the pool.
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	return p.rdb.SCard(ctx, p.indexKey()).Result()
}

// StartRefresh starts the periodic image refresh job.
func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration) {
	p.logger.Info("starting image pool refresh job",
		zap.Duration("interval", interval),
		zap.Int("pool_size", p.poolSize))

	if err := p.RefreshNow(ctx); err != nil {
		p.logger.Error("initial image refresh failed", zap.Error(err))
	}

	p.refreshTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-p.refreshTicker.C:
				if err := p.RefreshNow(ctx); err != nil {
					p.logger.Error("scheduled image refresh failed", zap.Error(err))
				}
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
	close(p.stopChan)
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

func (p *RedisImagePool) imageKey(imageID string) string {
	return fmt.Sprintf("captcha:images:%s", imageID)
}

func (p *RedisImagePool) indexKey() string {
	return "captcha:images:index"
}
