package application

import (
	"context"
	"errors"
	"testing"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

func TestShouldRefreshImagePoolOnStartup(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		existingCount int64
		want          bool
	}{
		{
			name:          "empty pool refreshes on startup",
			existingCount: 0,
			want:          true,
		},
		{
			name:          "non-empty pool skips startup refresh",
			existingCount: 1,
			want:          false,
		},
		{
			name:          "larger pool skips startup refresh",
			existingCount: 100,
			want:          false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ShouldRefreshImagePoolOnStartup(tc.existingCount); got != tc.want {
				t.Fatalf("ShouldRefreshImagePoolOnStartup(%d) = %v, want %v", tc.existingCount, got, tc.want)
			}
		})
	}
}

func TestCaptchaLifecycleStartImageRefresh(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		opts                 LifecycleOptions
		pool                 *fakeLifecycleImagePool
		wantSnapshotCalls    int
		wantStartCalls       int
		wantRefreshOnStartup bool
	}{
		{
			name: "disabled pool does not start",
			opts: LifecycleOptions{
				ImagePoolEnabled:      false,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: true,
			},
			pool:              &fakeLifecycleImagePool{},
			wantSnapshotCalls: 0,
			wantStartCalls:    0,
		},
		{
			name: "empty pool refreshes immediately",
			opts: LifecycleOptions{
				ImagePoolEnabled:      true,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: true,
			},
			pool: &fakeLifecycleImagePool{
				snapshot: domain.ImagePoolSnapshot{ImageCount: 0},
			},
			wantSnapshotCalls:    1,
			wantStartCalls:       1,
			wantRefreshOnStartup: true,
		},
		{
			name: "non-empty pool skips immediate refresh",
			opts: LifecycleOptions{
				ImagePoolEnabled:      true,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: true,
			},
			pool: &fakeLifecycleImagePool{
				snapshot: domain.ImagePoolSnapshot{ImageCount: 3},
			},
			wantSnapshotCalls:    1,
			wantStartCalls:       1,
			wantRefreshOnStartup: false,
		},
		{
			name: "snapshot error refreshes immediately",
			opts: LifecycleOptions{
				ImagePoolEnabled:      true,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: true,
			},
			pool: &fakeLifecycleImagePool{
				snapshotErr: errors.New("snapshot failed"),
			},
			wantSnapshotCalls:    1,
			wantStartCalls:       1,
			wantRefreshOnStartup: true,
		},
		{
			name: "probe disabled starts without snapshot",
			opts: LifecycleOptions{
				ImagePoolEnabled:      true,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: false,
			},
			pool:                 &fakeLifecycleImagePool{},
			wantSnapshotCalls:    0,
			wantStartCalls:       1,
			wantRefreshOnStartup: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lifecycle := NewCaptchaLifecycle(tc.pool, tc.opts, nil)
			if err := lifecycle.StartImageRefresh(context.Background()); err != nil {
				t.Fatalf("StartImageRefresh() error = %v", err)
			}

			if tc.pool.snapshotCalls != tc.wantSnapshotCalls {
				t.Fatalf("snapshot calls = %d, want %d", tc.pool.snapshotCalls, tc.wantSnapshotCalls)
			}
			if tc.pool.startCalls != tc.wantStartCalls {
				t.Fatalf("start calls = %d, want %d", tc.pool.startCalls, tc.wantStartCalls)
			}
			if tc.pool.startCalls > 0 {
				if tc.pool.startedInterval != tc.opts.ImageRefreshInterval {
					t.Fatalf("started interval = %v, want %v", tc.pool.startedInterval, tc.opts.ImageRefreshInterval)
				}
				if tc.pool.startedRefreshOnStartup != tc.wantRefreshOnStartup {
					t.Fatalf("refreshOnStartup = %v, want %v", tc.pool.startedRefreshOnStartup, tc.wantRefreshOnStartup)
				}
			}
		})
	}
}

func TestCaptchaLifecycleStopImageRefresh(t *testing.T) {
	t.Parallel()

	pool := &fakeLifecycleImagePool{}
	lifecycle := NewCaptchaLifecycle(pool, LifecycleOptions{ImagePoolEnabled: true}, nil)

	lifecycle.StopImageRefresh()

	if pool.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", pool.stopCalls)
	}
}

type fakeLifecycleImagePool struct {
	snapshot                domain.ImagePoolSnapshot
	snapshotErr             error
	snapshotCalls           int
	startCalls              int
	stopCalls               int
	startedInterval         time.Duration
	startedRefreshOnStartup bool
}

var _ appports.BackgroundImagePool = (*fakeLifecycleImagePool)(nil)

func (p *fakeLifecycleImagePool) Random(context.Context) ([]byte, error) {
	return nil, nil
}

func (p *fakeLifecycleImagePool) Snapshot(context.Context) (domain.ImagePoolSnapshot, error) {
	p.snapshotCalls++
	if p.snapshotErr != nil {
		return domain.ImagePoolSnapshot{}, p.snapshotErr
	}
	return p.snapshot, nil
}

func (p *fakeLifecycleImagePool) Refresh(context.Context) error {
	return nil
}

func (p *fakeLifecycleImagePool) RefreshWithProvider(context.Context, appports.ImageProvider) error {
	return nil
}

func (p *fakeLifecycleImagePool) Start(_ context.Context, interval time.Duration, refreshOnStartup bool) {
	p.startCalls++
	p.startedInterval = interval
	p.startedRefreshOnStartup = refreshOnStartup
}

func (p *fakeLifecycleImagePool) Stop() {
	p.stopCalls++
}
