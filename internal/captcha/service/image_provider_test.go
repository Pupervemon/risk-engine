package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

func TestRedisImagePoolReturnsErrorWhenRepositoryMissing(t *testing.T) {
	t.Parallel()

	pool := newRedisImagePool(nil, nil, nil, 1)

	_, err := pool.GetRandom(context.Background())
	if err == nil {
		t.Fatal("GetRandom() error = nil, want repository error")
	}
	if !strings.Contains(err.Error(), "image pool repository is not configured") {
		t.Fatalf("GetRandom() error = %q, want repository configuration error", err.Error())
	}
}

func TestRedisImagePoolLoadImagesUsesRepository(t *testing.T) {
	t.Parallel()

	repository := &fakeImagePoolRepository{}
	pool := newRedisImagePool(repository, nil, nil, 1)

	images := []ImageMeta{
		{ID: "img-1", Data: []byte("image-data"), URL: "https://example.test/img-1.jpg"},
	}
	if err := pool.LoadImages(context.Background(), images); err != nil {
		t.Fatalf("LoadImages() error = %v", err)
	}

	if repository.loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", repository.loadCalls)
	}
	if repository.cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", repository.cleanupCalls)
	}
	if repository.loadedGeneration == "" {
		t.Fatal("loaded generation is empty")
	}
	if got := repository.loadedImages[0]; got.ID != images[0].ID || string(got.Data) != string(images[0].Data) || got.URL != images[0].URL {
		t.Fatalf("loaded image = %+v, want %+v", got, images[0])
	}
}

type fakeImagePoolRepository struct {
	loadCalls        int
	cleanupCalls     int
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

func (r *fakeImagePoolRepository) LoadImagesIntoGeneration(_ context.Context, generation string, images []domain.ImageMeta) (string, error) {
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
	return true, nil
}

func (r *fakeImagePoolRepository) ReleaseRefreshLock(context.Context, string) error {
	return nil
}
