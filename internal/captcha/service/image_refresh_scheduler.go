package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type imagePoolRefreshScheduler struct {
	logger        *zap.Logger
	poolSize      int
	refresh       func(context.Context) error
	refreshTicker *time.Ticker
	midnightTimer *time.Timer
	stopChan      chan struct{}
	stopOnce      sync.Once
}

func newImagePoolRefreshScheduler(logger *zap.Logger, poolSize int, refresh func(context.Context) error) *imagePoolRefreshScheduler {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &imagePoolRefreshScheduler{
		logger:   logger,
		poolSize: poolSize,
		refresh:  refresh,
		stopChan: make(chan struct{}),
	}
}

func (s *imagePoolRefreshScheduler) Start(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if s == nil {
		return
	}

	nextMidnightAt := nextMidnightRefreshTime(time.Now())
	s.logger.Info("starting image pool refresh job",
		zap.Duration("interval", interval),
		zap.Int("pool_size", s.poolSize),
		zap.Bool("refresh_on_startup", refreshOnStartup),
		zap.Time("next_midnight_refresh_at", nextMidnightAt))

	if refreshOnStartup {
		if err := s.refreshNow(ctx); err != nil {
			s.logger.Error("initial image refresh failed", zap.Error(err))
		}
	} else {
		s.logger.Info("skipping initial image refresh because the pool already has cached images")
	}

	if interval > 0 {
		s.refreshTicker = time.NewTicker(interval)
	}
	s.midnightTimer = time.NewTimer(nextMidnightRefreshDelay(time.Now()))

	go s.run(ctx)
}

func (s *imagePoolRefreshScheduler) Stop() {
	if s == nil {
		return
	}

	if s.refreshTicker != nil {
		s.refreshTicker.Stop()
	}
	if s.midnightTimer != nil {
		s.midnightTimer.Stop()
	}
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

func (s *imagePoolRefreshScheduler) run(ctx context.Context) {
	for {
		var intervalChan <-chan time.Time
		if s.refreshTicker != nil {
			intervalChan = s.refreshTicker.C
		}

		var midnightChan <-chan time.Time
		if s.midnightTimer != nil {
			midnightChan = s.midnightTimer.C
		}

		select {
		case <-intervalChan:
			if err := s.refreshNow(ctx); err != nil {
				s.logger.Error("scheduled image refresh failed", zap.Error(err))
			}
		case <-midnightChan:
			if err := s.refreshNow(ctx); err != nil {
				s.logger.Error("midnight image refresh failed", zap.Error(err))
			}

			nextMidnightAt := nextMidnightRefreshTime(time.Now())
			s.logger.Info("scheduled next midnight image refresh",
				zap.Time("next_midnight_refresh_at", nextMidnightAt))
			s.midnightTimer.Reset(nextMidnightRefreshDelay(time.Now()))
		case <-s.stopChan:
			s.logger.Info("image pool refresh job stopped")
			return
		}
	}
}

func (s *imagePoolRefreshScheduler) refreshNow(ctx context.Context) error {
	if s.refresh == nil {
		return nil
	}
	return s.refresh(ctx)
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
