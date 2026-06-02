package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/redis/go-redis/v9"
)

const defaultImagePoolGenerationsToKeep = 3

var releaseImagePoolRefreshLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

// ImagePoolRepository stores captcha background-image generations in Redis.
type ImagePoolRepository struct {
	rdb *redis.Client
}

func NewImagePoolRepository(rdb *redis.Client) *ImagePoolRepository {
	if rdb == nil {
		return nil
	}
	return &ImagePoolRepository{rdb: rdb}
}

func (r *ImagePoolRepository) Random(ctx context.Context) ([]byte, error) {
	if r == nil || r.rdb == nil {
		return nil, fmt.Errorf("image pool repository is not configured")
	}

	generation, err := r.activeGeneration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active image generation: %w", err)
	}
	if generation == "" {
		return nil, fmt.Errorf("image pool is empty")
	}

	imageID, err := r.rdb.SRandMember(ctx, generationIndexKey(generation)).Result()
	if err == redis.Nil || imageID == "" {
		return nil, fmt.Errorf("image pool is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get image ID: %w", err)
	}

	data, err := r.rdb.Get(ctx, generationImageKey(generation, imageID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get image data: %w", err)
	}

	return data, nil
}

func (r *ImagePoolRepository) Count(ctx context.Context) (int64, error) {
	if r == nil || r.rdb == nil {
		return 0, fmt.Errorf("image pool repository is not configured")
	}

	generation, err := r.activeGeneration(ctx)
	if err != nil {
		return 0, err
	}
	if generation == "" {
		return 0, nil
	}

	return r.rdb.SCard(ctx, generationIndexKey(generation)).Result()
}

func (r *ImagePoolRepository) Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error) {
	if r == nil || r.rdb == nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("image pool repository is not configured")
	}

	generation, err := r.activeGeneration(ctx)
	if err != nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("get active image generation: %w", err)
	}

	snapshot := domain.ImagePoolSnapshot{
		ActiveGeneration: generation,
	}

	pipe := r.rdb.Pipeline()
	generationCountCmd := pipe.ZCard(ctx, generationsKey())

	var imageCountCmd *redis.IntCmd
	if generation != "" {
		imageCountCmd = pipe.SCard(ctx, generationIndexKey(generation))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("inspect image pool snapshot: %w", err)
	}

	snapshot.GenerationCount = generationCountCmd.Val()
	if imageCountCmd != nil {
		snapshot.ImageCount = imageCountCmd.Val()
	}
	if generation != "" {
		meta, err := r.loadGenerationMeta(ctx, generation)
		if err != nil {
			return domain.ImagePoolSnapshot{}, err
		}
		snapshot.SourceConfigVersion = meta.SourceConfigVersion
		snapshot.SourceURL = meta.SourceURL
		snapshot.RefreshedAt = meta.CreatedAt
		if meta.ImageCount > 0 {
			snapshot.ImageCount = meta.ImageCount
		}
	}

	return snapshot, nil
}

type imagePoolGenerationMetaPayload struct {
	Generation          string `json:"generation"`
	SourceConfigVersion int64  `json:"sourceConfigVersion"`
	SourceURL           string `json:"sourceURL"`
	ImageCount          int64  `json:"imageCount"`
	CreatedAt           string `json:"createdAt"`
}

func (r *ImagePoolRepository) LoadImagesIntoGeneration(ctx context.Context, generation string, images []domain.ImageMeta, meta domain.ImagePoolGenerationMeta) (string, error) {
	if r == nil || r.rdb == nil {
		return "", fmt.Errorf("image pool repository is not configured")
	}
	if generation == "" {
		return "", fmt.Errorf("image pool generation is required")
	}
	if len(images) == 0 {
		return "", fmt.Errorf("no images to load")
	}

	previousGeneration, err := r.activeGeneration(ctx)
	if err != nil {
		return "", fmt.Errorf("get active generation before refresh: %w", err)
	}

	pipe := r.rdb.Pipeline()
	for _, img := range images {
		pipe.Set(ctx, generationImageKey(generation, img.ID), img.Data, 0)
		pipe.SAdd(ctx, generationIndexKey(generation), img.ID)
	}
	if err := queueGenerationMeta(ctx, pipe, generation, meta); err != nil {
		return previousGeneration, err
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return previousGeneration, err
	}

	if err := r.activateGeneration(ctx, generation); err != nil {
		_ = r.deleteGeneration(ctx, generation)
		return previousGeneration, fmt.Errorf("activate image generation %s: %w", generation, err)
	}

	return previousGeneration, nil
}

func (r *ImagePoolRepository) CleanupStaleGenerations(ctx context.Context, generationsToKeep int) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	if generationsToKeep <= 0 {
		generationsToKeep = defaultImagePoolGenerationsToKeep
	}

	generations, err := r.rdb.ZRange(ctx, generationsKey(), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("list image generations: %w", err)
	}
	if len(generations) <= generationsToKeep {
		return nil
	}

	staleGenerations := generations[:len(generations)-generationsToKeep]
	for _, generation := range staleGenerations {
		if generation == "" {
			continue
		}
		if err := r.deleteGeneration(ctx, generation); err != nil {
			return err
		}
		if err := r.rdb.ZRem(ctx, generationsKey(), generation).Err(); err != nil {
			return fmt.Errorf("remove stale image generation %s: %w", generation, err)
		}
	}

	return nil
}

func (r *ImagePoolRepository) AcquireRefreshLock(ctx context.Context, token string, ttl time.Duration) (bool, error) {
	if r == nil || r.rdb == nil {
		return false, fmt.Errorf("image pool repository is not configured")
	}

	ok, err := r.rdb.SetNX(ctx, refreshLockKey(), token, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("acquire image pool refresh lock: %w", err)
	}
	return ok, nil
}

func (r *ImagePoolRepository) ReleaseRefreshLock(ctx context.Context, token string) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("image pool repository is not configured")
	}
	if token == "" {
		return nil
	}

	return releaseImagePoolRefreshLockScript.Run(ctx, r.rdb, []string{refreshLockKey()}, token).Err()
}

func (r *ImagePoolRepository) activateGeneration(ctx context.Context, generation string) error {
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, activeGenerationKey(), generation, 0)
	pipe.ZAdd(ctx, generationsKey(), redis.Z{
		Score:  float64(time.Now().UnixNano()),
		Member: generation,
	})

	_, err := pipe.Exec(ctx)
	return err
}

func queueGenerationMeta(ctx context.Context, pipe redis.Pipeliner, generation string, meta domain.ImagePoolGenerationMeta) error {
	meta.Generation = generation
	if meta.CreatedAt == "" {
		meta.CreatedAt = time.Now().Format(time.RFC3339)
	}

	payload := imagePoolGenerationMetaPayload{
		Generation:          meta.Generation,
		SourceConfigVersion: meta.SourceConfigVersion,
		SourceURL:           meta.SourceURL,
		ImageCount:          meta.ImageCount,
		CreatedAt:           meta.CreatedAt,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode image generation meta: %w", err)
	}

	pipe.Set(ctx, generationMetaKey(generation), encoded, 0)
	return nil
}

func (r *ImagePoolRepository) loadGenerationMeta(ctx context.Context, generation string) (domain.ImagePoolGenerationMeta, error) {
	if generation == "" {
		return domain.ImagePoolGenerationMeta{}, nil
	}

	value, err := r.rdb.Get(ctx, generationMetaKey(generation)).Bytes()
	if err == redis.Nil {
		return domain.ImagePoolGenerationMeta{}, nil
	}
	if err != nil {
		return domain.ImagePoolGenerationMeta{}, fmt.Errorf("get image generation meta: %w", err)
	}

	var payload imagePoolGenerationMetaPayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return domain.ImagePoolGenerationMeta{}, fmt.Errorf("decode image generation meta: %w", err)
	}

	return domain.ImagePoolGenerationMeta{
		Generation:          payload.Generation,
		SourceConfigVersion: payload.SourceConfigVersion,
		SourceURL:           payload.SourceURL,
		ImageCount:          payload.ImageCount,
		CreatedAt:           payload.CreatedAt,
	}, nil
}

func (r *ImagePoolRepository) deleteGeneration(ctx context.Context, generation string) error {
	ids, err := r.rdb.SMembers(ctx, generationIndexKey(generation)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("list image IDs for generation %s: %w", generation, err)
	}

	pipe := r.rdb.Pipeline()
	for _, imageID := range ids {
		pipe.Del(ctx, generationImageKey(generation, imageID))
	}
	pipe.Del(ctx, generationIndexKey(generation))
	pipe.Del(ctx, generationMetaKey(generation))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete image generation %s: %w", generation, err)
	}

	return nil
}

func (r *ImagePoolRepository) activeGeneration(ctx context.Context) (string, error) {
	generation, err := r.rdb.Get(ctx, activeGenerationKey()).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return generation, nil
}

func activeGenerationKey() string {
	return "captcha:images:active_generation"
}

func generationsKey() string {
	return "captcha:images:generations"
}

func generationImageKey(generation, imageID string) string {
	return fmt.Sprintf("captcha:images:g:%s:data:%s", generation, imageID)
}

func generationIndexKey(generation string) string {
	return fmt.Sprintf("captcha:images:g:%s:index", generation)
}

func generationMetaKey(generation string) string {
	return fmt.Sprintf("captcha:images:g:%s:meta", generation)
}

func refreshLockKey() string {
	return "captcha:images:refresh_lock"
}
