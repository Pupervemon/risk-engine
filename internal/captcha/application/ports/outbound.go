package ports

import (
	"context"
	"time"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type CaptchaAnswerRepository interface {
	Save(ctx context.Context, id string, answer domain.SliderAnswer, ttl time.Duration) error
	Get(ctx context.Context, id string) (domain.SliderAnswer, error)
	Delete(ctx context.Context, id string) error
}

type TokenRepository interface {
	Save(ctx context.Context, tokenDigest string, payload []byte, ttl time.Duration) error
	Exists(ctx context.Context, tokenDigest string) (bool, error)
}

type SliderGenerator interface {
	Generate(ctx context.Context, background []byte) (domain.GeneratedSlider, error)
}

type BackgroundImagePool interface {
	Random(ctx context.Context) ([]byte, error)
	Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error)
	Refresh(ctx context.Context) error
	RefreshWithProvider(ctx context.Context, provider ImageProvider) error
	Start(ctx context.Context, interval time.Duration, refreshOnStartup bool)
	Stop()
}

type ImageProvider interface {
	FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error)
}

type RuntimeImageSourceStore interface {
	Load(ctx context.Context) (domain.ImageSourceRuntimeConfig, bool, error)
	Save(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) error
}
