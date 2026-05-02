package application

import (
	"context"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"go.uber.org/zap"
)

type LifecycleOptions struct {
	ImagePoolEnabled       bool
	ImageRefreshInterval  time.Duration
	RefreshOnStartupProbe bool
}

type CaptchaLifecycle struct {
	imagePool appports.BackgroundImagePool
	opts      LifecycleOptions
	logger    *zap.Logger
}

var _ appports.CaptchaLifecycle = (*CaptchaLifecycle)(nil)

func NewCaptchaLifecycle(imagePool appports.BackgroundImagePool, opts LifecycleOptions, logger *zap.Logger) *CaptchaLifecycle {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CaptchaLifecycle{
		imagePool: imagePool,
		opts:      opts,
		logger:    logger,
	}
}

func (l *CaptchaLifecycle) StartImageRefresh(ctx context.Context) error {
	if l == nil || !l.opts.ImagePoolEnabled || l.imagePool == nil {
		l.logger.Info("image pool is disabled, skipping refresh job")
		return nil
	}

	refreshOnStartup := true
	if l.opts.RefreshOnStartupProbe {
		snapshot, err := l.imagePool.Snapshot(ctx)
		if err != nil {
			l.logger.Warn("failed to inspect image pool before startup refresh; falling back to immediate refresh",
				zap.Error(err))
		} else if !ShouldRefreshImagePoolOnStartup(snapshot.ImageCount) {
			refreshOnStartup = false
			l.logger.Info("image pool already contains cached images; skipping immediate startup refresh",
				zap.Int64("current_count", snapshot.ImageCount))
		} else {
			l.logger.Info("image pool is empty; performing immediate startup refresh",
				zap.Int64("current_count", snapshot.ImageCount))
		}
	}

	l.imagePool.Start(ctx, l.opts.ImageRefreshInterval, refreshOnStartup)
	l.logger.Info("image pool refresh job started",
		zap.Duration("interval", l.opts.ImageRefreshInterval))

	return nil
}

func (l *CaptchaLifecycle) StopImageRefresh() {
	if l == nil || l.imagePool == nil {
		return
	}

	l.imagePool.Stop()
	l.logger.Info("image pool refresh job stopped")
}

func ShouldRefreshImagePoolOnStartup(existingCount int64) bool {
	return existingCount <= 0
}
