package imagepool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"go.uber.org/zap"
)

type imagePoolRefresher struct {
	logger     *zap.Logger
	repository imagePoolRepository
	poolSize   int
}

func newImagePoolRefresher(repository imagePoolRepository, logger *zap.Logger, poolSize int) *imagePoolRefresher {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &imagePoolRefresher{
		logger:     logger,
		repository: repository,
		poolSize:   poolSize,
	}
}

func (r *imagePoolRefresher) Refresh(ctx context.Context, provider ImageProvider, meta domain.ImagePoolGenerationMeta) error {
	if provider == nil {
		return fmt.Errorf("image provider is not configured")
	}
	if r == nil || r.repository == nil {
		return fmt.Errorf("image pool repository is not configured")
	}

	lockToken, err := r.acquireRefreshLock(ctx)
	if err != nil {
		return err
	}
	defer r.releaseRefreshLock(lockToken)

	r.logger.Info("refreshing image pool", zap.Int("target_size", r.poolSize))
	startTime := time.Now()

	oldCount, _ := r.repository.Count(ctx)

	images, err := provider.FetchImages(ctx, r.poolSize)
	if err != nil {
		return fmt.Errorf("failed to fetch images: %w", err)
	}

	if err := r.loadImages(ctx, images, meta); err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	newCount, _ := r.repository.Count(ctx)

	r.logger.Info("image pool refreshed",
		zap.Int64("old_count", oldCount),
		zap.Int64("new_count", newCount),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

func (r *imagePoolRefresher) loadImages(ctx context.Context, images []ImageMeta, meta domain.ImagePoolGenerationMeta) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to load")
	}

	generation, err := newImagePoolGeneration()
	if err != nil {
		return fmt.Errorf("generate image pool generation: %w", err)
	}

	return r.loadImagesIntoGeneration(ctx, generation, images, meta)
}

func (r *imagePoolRefresher) loadImagesIntoGeneration(ctx context.Context, generation string, images []ImageMeta, meta domain.ImagePoolGenerationMeta) error {
	if generation == "" {
		return fmt.Errorf("image pool generation is required")
	}

	r.logger.Info("loading images into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation))
	startTime := time.Now()

	meta.Generation = generation
	meta.ImageCount = int64(len(images))
	if meta.CreatedAt == "" {
		meta.CreatedAt = time.Now().Format(time.RFC3339)
	}

	previousGeneration, err := r.repository.LoadImagesIntoGeneration(ctx, generation, images, meta)
	if err != nil {
		r.logger.Error("failed to load images into redis",
			zap.Error(err),
			zap.String("generation", generation))
		return err
	}

	if err := r.repository.CleanupStaleGenerations(ctx, imagePoolGenerationsToKeep); err != nil {
		r.logger.Warn("failed to cleanup stale image generations", zap.Error(err))
	}

	r.logger.Info("images loaded into redis",
		zap.Int("count", len(images)),
		zap.String("generation", generation),
		zap.String("previous_generation", previousGeneration),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

func (r *imagePoolRefresher) acquireRefreshLock(ctx context.Context) (string, error) {
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
		ok, err := r.repository.AcquireRefreshLock(lockCtx, token, imagePoolRefreshLockTTL)
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

func (r *imagePoolRefresher) releaseRefreshLock(token string) {
	if token == "" {
		return
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := r.repository.ReleaseRefreshLock(releaseCtx, token); err != nil {
		r.logger.Warn("failed to release image pool refresh lock", zap.Error(err))
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
