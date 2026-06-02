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
		imageSource          *fakeLifecycleImageSource
		wantSnapshotCalls    int
		wantRefreshOnStartup bool
		wantRefreshCalls     int
	}{
		{
			name: "disabled pool does not start",
			opts: LifecycleOptions{
				ImagePoolEnabled:      false,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: true,
			},
			pool:              &fakeLifecycleImagePool{},
			imageSource:       &fakeLifecycleImageSource{},
			wantSnapshotCalls: 0,
			wantRefreshCalls:  0,
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
			imageSource:          &fakeLifecycleImageSource{},
			wantSnapshotCalls:    1,
			wantRefreshOnStartup: true,
			wantRefreshCalls:     1,
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
			imageSource:          &fakeLifecycleImageSource{},
			wantSnapshotCalls:    1,
			wantRefreshOnStartup: false,
			wantRefreshCalls:     0,
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
			imageSource:          &fakeLifecycleImageSource{},
			wantSnapshotCalls:    1,
			wantRefreshOnStartup: true,
			wantRefreshCalls:     1,
		},
		{
			name: "probe disabled starts without snapshot",
			opts: LifecycleOptions{
				ImagePoolEnabled:      true,
				ImageRefreshInterval:  time.Minute,
				RefreshOnStartupProbe: false,
			},
			imageSource:          &fakeLifecycleImageSource{},
			pool:                 &fakeLifecycleImagePool{},
			wantSnapshotCalls:    0,
			wantRefreshOnStartup: true,
			wantRefreshCalls:     1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lifecycle := NewCaptchaLifecycle(tc.pool, tc.imageSource, tc.opts, nil)
			defer lifecycle.StopImageRefresh()

			if err := lifecycle.StartImageRefresh(context.Background()); err != nil {
				t.Fatalf("StartImageRefresh() error = %v", err)
			}

			if tc.pool.snapshotCalls != tc.wantSnapshotCalls {
				t.Fatalf("snapshot calls = %d, want %d", tc.pool.snapshotCalls, tc.wantSnapshotCalls)
			}
			if tc.imageSource.refreshCalls != tc.wantRefreshCalls {
				t.Fatalf("refresh calls = %d, want %d", tc.imageSource.refreshCalls, tc.wantRefreshCalls)
			}
			if got := tc.imageSource.refreshCalls > 0; got != tc.wantRefreshOnStartup {
				t.Fatalf("refresh on startup = %v, want %v", got, tc.wantRefreshOnStartup)
			}
		})
	}
}

func TestCaptchaLifecycleStopImageRefresh(t *testing.T) {
	t.Parallel()

	pool := &fakeLifecycleImagePool{}
	lifecycle := NewCaptchaLifecycle(pool, &fakeLifecycleImageSource{}, LifecycleOptions{ImagePoolEnabled: true}, nil)

	lifecycle.StopImageRefresh()
}

func TestCaptchaLifecycleRequiresImageSourceWhenPoolEnabled(t *testing.T) {
	t.Parallel()

	lifecycle := NewCaptchaLifecycle(&fakeLifecycleImagePool{}, nil, LifecycleOptions{ImagePoolEnabled: true}, nil)

	if err := lifecycle.StartImageRefresh(context.Background()); err == nil {
		t.Fatal("StartImageRefresh() error = nil, want missing image source error")
	}
}

func TestNextMidnightRefreshTime(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.April, 22, 15, 4, 5, 0, location)

	got := nextMidnightRefreshTime(now)
	want := time.Date(2026, time.April, 23, 0, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("nextMidnightRefreshTime(%v) = %v, want %v", now, got, want)
	}
}

func TestNextMidnightRefreshTimeAcrossYearBoundary(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC", 0)
	now := time.Date(2026, time.December, 31, 23, 59, 59, 0, location)

	got := nextMidnightRefreshTime(now)
	want := time.Date(2027, time.January, 1, 0, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("nextMidnightRefreshTime(%v) = %v, want %v", now, got, want)
	}
}

func TestNextMidnightRefreshDelay(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.April, 22, 23, 59, 30, 0, location)

	got := nextMidnightRefreshDelay(now)
	want := 30 * time.Second
	if got != want {
		t.Fatalf("nextMidnightRefreshDelay(%v) = %v, want %v", now, got, want)
	}
}

type fakeLifecycleImagePool struct {
	snapshot      domain.ImagePoolSnapshot
	snapshotErr   error
	snapshotCalls int
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

func (p *fakeLifecycleImagePool) RefreshWithProvider(context.Context, appports.ImageProvider, domain.ImagePoolGenerationMeta) error {
	return nil
}

type fakeLifecycleImageSource struct {
	refreshCalls int
}

var _ appports.ImageSourceUseCase = (*fakeLifecycleImageSource)(nil)

func (s *fakeLifecycleImageSource) Status(context.Context) (domain.ImageSourceStatus, error) {
	return domain.ImageSourceStatus{}, nil
}

func (s *fakeLifecycleImageSource) Check(context.Context) (domain.ImageSourceValidationResult, error) {
	return domain.ImageSourceValidationResult{}, nil
}

func (s *fakeLifecycleImageSource) Update(context.Context, domain.ImageSourcePatch, bool) (domain.ImageSourceStatus, error) {
	return domain.ImageSourceStatus{}, nil
}

func (s *fakeLifecycleImageSource) Refresh(context.Context) (domain.ImageSourceStatus, error) {
	s.refreshCalls++
	return domain.ImageSourceStatus{}, nil
}
