package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"go.uber.org/zap"
)

type LifecycleOptions struct {
	ImagePoolEnabled      bool
	ImageRefreshInterval  time.Duration
	RefreshOnStartupProbe bool
}

type CaptchaLifecycle struct {
	imagePool    appports.BackgroundImagePool
	imageSource  appports.ImageSourceUseCase
	opts         LifecycleOptions
	logger       *zap.Logger
	refreshTick  *time.Ticker
	midnightTick *time.Timer
	stopChan     chan struct{}
	stopOnce     sync.Once
}

var _ appports.CaptchaLifecycle = (*CaptchaLifecycle)(nil)

func NewCaptchaLifecycle(
	imagePool appports.BackgroundImagePool,
	imageSource appports.ImageSourceUseCase,
	opts LifecycleOptions,
	logger *zap.Logger,
) *CaptchaLifecycle {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CaptchaLifecycle{
		imagePool:   imagePool,
		imageSource: imageSource,
		opts:        opts,
		logger:      logger,
		stopChan:    make(chan struct{}),
	}
}

func (l *CaptchaLifecycle) StartImageRefresh(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if !l.opts.ImagePoolEnabled || l.imagePool == nil {
		l.logger.Info("image pool is disabled, skipping refresh job")
		return nil
	}
	if l.imageSource == nil {
		return fmt.Errorf("image source use case is not configured")
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

	l.startRefreshJob(ctx, refreshOnStartup)
	l.logger.Info("image pool refresh job started",
		zap.Duration("interval", l.opts.ImageRefreshInterval))

	return nil
}

func (l *CaptchaLifecycle) StopImageRefresh() {
	if l == nil {
		return
	}

	if l.refreshTick != nil {
		l.refreshTick.Stop()
	}
	if l.midnightTick != nil {
		l.midnightTick.Stop()
	}
	l.stopOnce.Do(func() {
		close(l.stopChan)
	})
	l.logger.Info("image pool refresh job stopped")
}

func ShouldRefreshImagePoolOnStartup(existingCount int64) bool {
	return existingCount <= 0
}

func (l *CaptchaLifecycle) startRefreshJob(ctx context.Context, refreshOnStartup bool) {
	nextMidnightAt := nextMidnightRefreshTime(time.Now())
	l.logger.Info("starting image pool refresh job",
		zap.Duration("interval", l.opts.ImageRefreshInterval),
		zap.Bool("refresh_on_startup", refreshOnStartup),
		zap.Time("next_midnight_refresh_at", nextMidnightAt))

	if refreshOnStartup {
		if err := l.refreshNow(ctx); err != nil {
			l.logger.Error("initial image refresh failed", zap.Error(err))
		}
	} else {
		l.logger.Info("skipping initial image refresh because the pool already has cached images")
	}

	if l.opts.ImageRefreshInterval > 0 {
		l.refreshTick = time.NewTicker(l.opts.ImageRefreshInterval)
	}
	l.midnightTick = time.NewTimer(nextMidnightRefreshDelay(time.Now()))

	go l.runRefreshJob(ctx)
}

func (l *CaptchaLifecycle) runRefreshJob(ctx context.Context) {
	for {
		var refreshChan <-chan time.Time
		if l.refreshTick != nil {
			refreshChan = l.refreshTick.C
		}

		var midnightChan <-chan time.Time
		if l.midnightTick != nil {
			midnightChan = l.midnightTick.C
		}

		select {
		case <-refreshChan:
			if err := l.refreshNow(ctx); err != nil {
				l.logger.Error("scheduled image refresh failed", zap.Error(err))
			}
		case <-midnightChan:
			if err := l.refreshNow(ctx); err != nil {
				l.logger.Error("midnight image refresh failed", zap.Error(err))
			}
			nextMidnightAt := nextMidnightRefreshTime(time.Now())
			l.logger.Info("scheduled next midnight image refresh",
				zap.Time("next_midnight_refresh_at", nextMidnightAt))
			l.midnightTick.Reset(nextMidnightRefreshDelay(time.Now()))
		case <-l.stopChan:
			return
		}
	}
}

func (l *CaptchaLifecycle) refreshNow(ctx context.Context) error {
	if l == nil || l.imageSource == nil {
		return nil
	}
	_, err := l.imageSource.Refresh(ctx)
	return err
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
