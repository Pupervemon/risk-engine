package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ImageMeta 图片元数据
type ImageMeta struct {
	ID   string // 唯一标识
	Data []byte // 图片二进制数据
	URL  string // 原始URL（用于日志）
}

// ImageProvider 图片提供者接口
type ImageProvider interface {
	// FetchImages 从外部源批量获取图片
	FetchImages(ctx context.Context, count int) ([]ImageMeta, error)
}

// RedisImagePool Redis图片池
type RedisImagePool struct {
	rdb           *redis.Client
	logger        *zap.Logger
	provider      ImageProvider
	poolSize      int           // 图片池大小
	refreshTicker *time.Ticker  // 定时刷新任务
	stopChan      chan struct{} // 停止信号
}

// NewRedisImagePool 创建Redis图片池
func NewRedisImagePool(rdb *redis.Client, logger *zap.Logger, provider ImageProvider, poolSize int) *RedisImagePool {
	return &RedisImagePool{
		rdb:      rdb,
		logger:   logger,
		provider: provider,
		poolSize: poolSize,
		stopChan: make(chan struct{}),
	}
}

// LoadImages 批量加载图片到Redis
func (p *RedisImagePool) LoadImages(ctx context.Context, images []ImageMeta) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to load")
	}

	p.logger.Info("开始加载图片到Redis", zap.Int("count", len(images)))
	startTime := time.Now()

	pipe := p.rdb.Pipeline()

	// 清理旧的索引集合和图片数据
	oldIDs, err := p.rdb.SMembers(ctx, p.indexKey()).Result()
	if err == nil && len(oldIDs) > 0 {
		// 删除旧图片
		for _, oldID := range oldIDs {
			pipe.Del(ctx, p.imageKey(oldID))
		}
		pipe.Del(ctx, p.indexKey())
	}

	// 写入新图片
	for _, img := range images {
		pipe.Set(ctx, p.imageKey(img.ID), img.Data, 0) // 不设置过期时间
		pipe.SAdd(ctx, p.indexKey(), img.ID)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		p.logger.Error("加载图片到Redis失败", zap.Error(err))
		return err
	}

	p.logger.Info("图片加载完成",
		zap.Int("count", len(images)),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// GetRandom 随机获取一张图片
func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	// 获取所有图片ID
	ids, err := p.rdb.SMembers(ctx, p.indexKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get image IDs: %w", err)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("image pool is empty")
	}

	// 随机选择一个ID
	randomIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(ids))))
	if err != nil {
		return nil, fmt.Errorf("failed to generate random index: %w", err)
	}

	selectedID := ids[randomIdx.Int64()]

	// 获取图片数据
	data, err := p.rdb.Get(ctx, p.imageKey(selectedID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get image data: %w", err)
	}

	return data, nil
}

// Count 返回当前图片池大小
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	return p.rdb.SCard(ctx, p.indexKey()).Result()
}

// StartRefresh 启动定时刷新任务
func (p *RedisImagePool) StartRefresh(ctx context.Context, interval time.Duration) {
	p.logger.Info("启动图片池定时刷新任务",
		zap.Duration("interval", interval),
		zap.Int("pool_size", p.poolSize))

	// 立即执行一次加载
	if err := p.refresh(ctx); err != nil {
		p.logger.Error("初始图片加载失败", zap.Error(err))
	}

	// 启动定时任务
	p.refreshTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-p.refreshTicker.C:
				if err := p.refresh(ctx); err != nil {
					p.logger.Error("定时刷新图片失败", zap.Error(err))
				}
			case <-p.stopChan:
				p.logger.Info("图片池刷新任务已停止")
				return
			}
		}
	}()
}

// StopRefresh 停止定时刷新任务
func (p *RedisImagePool) StopRefresh() {
	if p.refreshTicker != nil {
		p.refreshTicker.Stop()
	}
	close(p.stopChan)
}

// refresh 执行图片刷新逻辑
func (p *RedisImagePool) refresh(ctx context.Context) error {
	p.logger.Info("开始刷新图片池", zap.Int("target_size", p.poolSize))
	startTime := time.Now()

	// 获取当前池大小
	oldCount, _ := p.Count(ctx)

	// 从外部源获取图片
	images, err := p.provider.FetchImages(ctx, p.poolSize)
	if err != nil {
		return fmt.Errorf("failed to fetch images: %w", err)
	}

	// 加载到Redis
	if err := p.LoadImages(ctx, images); err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	newCount, _ := p.Count(ctx)

	p.logger.Info("图片池刷新完成",
		zap.Int64("old_count", oldCount),
		zap.Int64("new_count", newCount),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// imageKey 生成图片数据的Redis key
func (p *RedisImagePool) imageKey(imageID string) string {
	return fmt.Sprintf("captcha:images:%s", imageID)
}

// indexKey 生成索引集合的Redis key
func (p *RedisImagePool) indexKey() string {
	return "captcha:images:index"
}
