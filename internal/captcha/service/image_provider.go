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
	// 刷新锁的默认存活时间，避免某个刷新任务异常退出后锁永久占用。
	imagePoolRefreshLockTTL = 15 * time.Minute
	// 获取刷新锁的最长等待时间；超过这个时间仍拿不到锁，则认为当前已有任务在刷新。
	imagePoolRefreshLockAcquireTimeout = 30 * time.Second
	// 获取锁失败时的重试间隔，避免 Redis 被高频轮询。
	imagePoolRefreshLockRetryInterval = 500 * time.Millisecond
	// 仅保留最近几代图片池，防止历史数据无限累积。
	imagePoolGenerationsToKeep = 3
)

var (
	// 当已有刷新任务持有锁时，新的刷新请求会返回这个错误。
	ErrImagePoolRefreshInProgress = errors.New("captcha image pool refresh is already in progress")

	// 使用 Lua 脚本保证“只删除自己持有的锁”，避免误删别的刷新任务的锁。
	releaseImagePoolRefreshLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)
)

// ImageMeta contains normalized image data stored in the captcha image pool.
type ImageMeta struct {
	// ID 是图片在当前批次中的唯一标识，用作 Redis 中的 key 后缀和集合成员。
	ID string
	// Data 是实际存储的图片二进制内容。
	Data []byte
	// URL 记录图片来源，便于排查和追踪上游资源。
	URL string
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
	// refreshMu 用于串行化同进程内的刷新调用，避免重复触发同一批刷新流程。
	refreshMu sync.Mutex
}

// ImagePoolSnapshot describes the current active image-pool generation state.
type ImagePoolSnapshot struct {
	// ImageCount 是当前激活代中可随机抽取的图片数量。
	ImageCount int64
	// ActiveGeneration 是当前对外生效的图片池代号。
	ActiveGeneration string
	// GenerationCount 是 Redis 中记录的历史代总数，用于观察清理是否生效。
	GenerationCount int64
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

	// 每次加载都生成新的代号，确保新旧图片池可以并存，直到新代完全切换成功。
	generation, err := newImagePoolGeneration()
	if err != nil {
		return fmt.Errorf("generate image pool generation: %w", err)
	}

	return p.loadImagesIntoGeneration(ctx, generation, images)
}

// GetRandom returns a random image from the active pool generation.
func (p *RedisImagePool) GetRandom(ctx context.Context) ([]byte, error) {
	// 先拿到当前生效的代号，再从这个代号对应的集合里随机取一个图片 ID。
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

	// 图片数据单独按 generation + imageID 存储，便于整体替换和后续清理。
	data, err := p.rdb.Get(ctx, p.generationImageKey(generation, imageID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get image data: %w", err)
	}

	return data, nil
}

// Count returns the current number of images in the active pool generation.
func (p *RedisImagePool) Count(ctx context.Context) (int64, error) {
	// 统计的是当前激活代的图片数，而不是历史累计数量。
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
	// 先读当前激活代，再按需统计该代图片数和所有代的总数。
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
	// 启动时记录下一次本地午夜刷新时间，便于定位定时任务行为。
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

	// 周期刷新器负责固定间隔的补充刷新；如果 interval <= 0，则不启用。
	if interval > 0 {
		p.refreshTicker = time.NewTicker(interval)
	}
	// 额外加一个本地午夜刷新，确保每天至少有一次更新机会。
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
				// 周期刷新失败只记录日志，不中断后续调度。
				if err := p.RefreshNow(ctx); err != nil {
					p.logger.Error("scheduled image refresh failed", zap.Error(err))
				}
			case <-midnightChan:
				// 午夜刷新完成后，重新计算下一次午夜的延迟并重置定时器。
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
	// 先停止本地定时器，避免后续继续触发新一轮刷新。
	if p.refreshTicker != nil {
		p.refreshTicker.Stop()
	}
	if p.midnightTimer != nil {
		p.midnightTimer.Stop()
	}
	// stopChan 只关闭一次，保证调用 StopRefresh 的幂等性。
	p.stopOnce.Do(func() {
		close(p.stopChan)
	})
}

// RefreshNow refreshes the pool using the active provider.
func (p *RedisImagePool) RefreshNow(ctx context.Context) error {
	// 这里使用当前配置的 provider，外部调用者无需关心数据来源实现。
	return p.RefreshWithProvider(ctx, p.provider)
}

// RefreshWithProvider refreshes the pool using a supplied provider without replacing the active one.
func (p *RedisImagePool) RefreshWithProvider(ctx context.Context, provider ImageProvider) error {
	if provider == nil {
		return fmt.Errorf("image provider is not configured")
	}

	// 进程内只允许一次刷新流程同时执行，避免多个定时器或外部调用并发抢占资源。
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	return p.refreshWithProvider(ctx, provider)
}

func (p *RedisImagePool) refreshWithProvider(ctx context.Context, provider ImageProvider) error {
	// Redis 锁是跨进程的保护，确保多个实例部署时只有一个实例真正刷新图片池。
	lockToken, err := p.acquireRefreshLock(ctx)
	if err != nil {
		return err
	}
	defer p.releaseRefreshLock(lockToken)

	p.logger.Info("refreshing image pool", zap.Int("target_size", p.poolSize))
	startTime := time.Now()

	// 记录刷新前的数量，便于观察刷新是否真正替换了图片池。
	oldCount, _ := p.Count(ctx)

	// 先从上游拉取足够数量的图片，再一次性写入 Redis。
	images, err := provider.FetchImages(ctx, p.poolSize)
	if err != nil {
		return fmt.Errorf("failed to fetch images: %w", err)
	}

	// 只有当整批图片都成功写入并切换代号后，新的图片池才会对外可见。
	if err := p.LoadImages(ctx, images); err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	// 刷新完成后再读一次数量，用于日志和排查实际加载结果。
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

	// 先把新代的数据写入 Redis，但此时还不切换 active generation，避免半成品数据被读到。
	p.logger.Info("loading images into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation))
	startTime := time.Now()

	// 记录旧代号，方便观察切换前后是否发生了轮换。
	previousGeneration, err := p.activeGeneration(ctx)
	if err != nil {
		return fmt.Errorf("get active generation before refresh: %w", err)
	}

	// 将图片二进制和索引分别写入：一个 key 存内容，一个集合存该代全部 ID。
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

	// 只有在数据完整落盘之后，才把这一代设置为 active。
	if err := p.activateGeneration(ctx, generation); err != nil {
		// 如果切换代号失败，清理刚写入的新代数据，避免留下孤儿数据。
		_ = p.deleteGeneration(ctx, generation)
		return fmt.Errorf("activate image generation %s: %w", generation, err)
	}

	// 清理较旧的历史代，控制 Redis 占用。
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
	// 使用事务管线同时更新 active generation 和历史代索引，尽量保持两者一致。
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
	// 按时间顺序取出所有代号，最早的就是最旧的历史数据。
	generations, err := p.rdb.ZRange(ctx, p.generationsKey(), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("list image generations: %w", err)
	}
	if len(generations) <= imagePoolGenerationsToKeep {
		// 历史代数量未超限时，不做任何清理。
		return nil
	}

	// 只保留最新的几代，前面的旧代逐个删除。
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
	// 先从索引集合里找出该代包含的所有图片 ID，再按 key 逐个删除内容数据。
	ids, err := p.rdb.SMembers(ctx, p.generationIndexKey(generation)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("list image IDs for generation %s: %w", generation, err)
	}

	// 索引集合本身也要删除，避免留下无法引用的历史记录。
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
	// active generation 是整个图片池对外可见的“当前版本号”。
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
	// 如果调用方没有 deadline，就补一个超时，防止无限等待刷新锁。
	lockCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		lockCtx, cancel = context.WithTimeout(ctx, imagePoolRefreshLockAcquireTimeout)
	}
	defer cancel()

	// token 作为锁的持有者标识，释放时必须和 Redis 中保存的一致。
	token, err := newImagePoolToken()
	if err != nil {
		return "", fmt.Errorf("generate refresh lock token: %w", err)
	}

	// 轮询尝试获取锁，直到成功或超时。
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

	// 释放锁时也设置一个短超时，避免因为 Redis 异常导致清理阶段卡住。
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
	// 按本地时区计算“明天 00:00”，保证定时逻辑符合部署机器所在时区。
	return time.Date(
		now.Year(),
		now.Month(),
		now.Day()+1,
		0, 0, 0, 0,
		now.Location(),
	)
}

func nextMidnightRefreshDelay(now time.Time) time.Duration {
	// 直接计算距离下一次本地午夜还有多久。
	return nextMidnightRefreshTime(now).Sub(now)
}

func newImagePoolGeneration() (string, error) {
	// 生成代号时使用时间戳 + 随机后缀，既能大致按时间排序，又能避免同一纳秒碰撞。
	suffix, err := newImagePoolToken()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), suffix), nil
}

func newImagePoolToken() (string, error) {
	// 使用足够长度的随机字节生成十六进制字符串，适合作为锁 token 或代号后缀。
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
