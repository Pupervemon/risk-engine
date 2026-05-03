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

type ManagedBackgroundImagePool interface {
	BackgroundImagePool
	Count(ctx context.Context) (int64, error)
	SetProvider(provider ImageProvider)
	PoolSize() int
}

type ImageProvider interface {
	FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error)
}

type ImageProviderFactory interface {
	BuildRuntimeProvider(cfg domain.ImageSourceRuntimeConfig) (ImageProvider, error)
}

type RuntimeImageSourceManager interface {
	BuildCandidateConfig(patch domain.ImageSourcePatch) (domain.ImageSourceRuntimeConfig, error)
	ValidateConfig(ctx context.Context, candidate domain.ImageSourceRuntimeConfig) (ImageProvider, error)
	ApplyConfig(candidate domain.ImageSourceRuntimeConfig, provider ImageProvider)
	RestoreConfig(candidate domain.ImageSourceRuntimeConfig, provider ImageProvider)
	BuildProvider(cfg domain.ImageSourceRuntimeConfig) (ImageProvider, error)
	RecordRefreshResult(err error)
	ValidationResult(candidate domain.ImageSourceRuntimeConfig) domain.ImageSourceValidationResult
	Status(poolSize int, poolSnapshot domain.ImagePoolSnapshot) domain.ImageSourceStatus
}

type RuntimeImageSourceStore interface {
	Load(ctx context.Context) (domain.ImageSourceRuntimeConfig, bool, error)
	Save(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) error
}
