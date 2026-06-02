package imagepool

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

func TestImagePoolReturnsErrorWhenRepositoryMissing(t *testing.T) {
	t.Parallel()

	pool := newImagePool(nil, nil, 1)

	_, err := pool.Random(context.Background())
	if err == nil {
		t.Fatal("Random() error = nil, want repository error")
	}
	if !strings.Contains(err.Error(), "image pool repository is not configured") {
		t.Fatalf("Random() error = %q, want repository configuration error", err.Error())
	}
}

func TestImagePoolRefreshWithProviderUsesLockAndProvider(t *testing.T) {
	t.Parallel()

	repository := &fakeImagePoolRepository{}
	pool := newImagePool(repository, nil, 2)
	provider := &fakeImageProvider{
		images: []domain.ImageMeta{
			{ID: "img-1", Data: []byte("image-1"), URL: "https://example.test/img-1.jpg"},
			{ID: "img-2", Data: []byte("image-2"), URL: "https://example.test/img-2.jpg"},
		},
	}

	if err := pool.RefreshWithProvider(context.Background(), provider, domain.ImagePoolGenerationMeta{
		SourceConfigVersion: 1,
		SourceURL:           "https://example.test/api",
	}); err != nil {
		t.Fatalf("RefreshWithProvider() error = %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if provider.requestedCount != 2 {
		t.Fatalf("provider requested count = %d, want 2", provider.requestedCount)
	}
	if repository.acquireLockCalls != 1 {
		t.Fatalf("acquire lock calls = %d, want 1", repository.acquireLockCalls)
	}
	if repository.releaseLockCalls != 1 {
		t.Fatalf("release lock calls = %d, want 1", repository.releaseLockCalls)
	}
	if repository.loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", repository.loadCalls)
	}
	if len(repository.loadedImages) != 2 {
		t.Fatalf("loaded images = %d, want 2", len(repository.loadedImages))
	}
}

func TestImagePoolRefreshInProgressUsesDomainError(t *testing.T) {
	t.Parallel()

	repository := &busyImagePoolRepository{}
	pool := newImagePool(repository, nil, 1)
	provider := &fakeImageProvider{
		images: []domain.ImageMeta{
			{ID: "img-1", Data: []byte("image-1"), URL: "https://example.test/img-1.jpg"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := pool.RefreshWithProvider(ctx, provider, domain.ImagePoolGenerationMeta{})
	if !errors.Is(err, domain.ErrImagePoolRefreshInProgress) {
		t.Fatalf("RefreshWithProvider() error = %v, want domain refresh-in-progress error", err)
	}
}

type fakeImagePoolRepository struct {
	loadCalls        int
	cleanupCalls     int
	acquireLockCalls int
	releaseLockCalls int
	loadedGeneration string
	loadedImages     []domain.ImageMeta
}

func (r *fakeImagePoolRepository) Random(context.Context) ([]byte, error) {
	return nil, nil
}

func (r *fakeImagePoolRepository) Count(context.Context) (int64, error) {
	return int64(len(r.loadedImages)), nil
}

func (r *fakeImagePoolRepository) Snapshot(context.Context) (domain.ImagePoolSnapshot, error) {
	return domain.ImagePoolSnapshot{ImageCount: int64(len(r.loadedImages))}, nil
}

func (r *fakeImagePoolRepository) LoadImagesIntoGeneration(_ context.Context, generation string, images []domain.ImageMeta, _ domain.ImagePoolGenerationMeta) (string, error) {
	r.loadCalls++
	r.loadedGeneration = generation
	r.loadedImages = append([]domain.ImageMeta(nil), images...)
	return "", nil
}

func (r *fakeImagePoolRepository) CleanupStaleGenerations(context.Context, int) error {
	r.cleanupCalls++
	return nil
}

func (r *fakeImagePoolRepository) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	r.acquireLockCalls++
	return true, nil
}

func (r *fakeImagePoolRepository) ReleaseRefreshLock(context.Context, string) error {
	r.releaseLockCalls++
	return nil
}

type busyImagePoolRepository struct {
	fakeImagePoolRepository
}

func (r *busyImagePoolRepository) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	r.acquireLockCalls++
	return false, nil
}

type fakeImageProvider struct {
	images         []domain.ImageMeta
	calls          int
	requestedCount int
}

func (p *fakeImageProvider) FetchImages(_ context.Context, count int) ([]domain.ImageMeta, error) {
	p.calls++
	p.requestedCount = count
	return p.images, nil
}
