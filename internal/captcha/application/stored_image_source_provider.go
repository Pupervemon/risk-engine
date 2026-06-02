package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// StoredImageSourceProvider reads the current image-source config from Redis
// on each image fetch and builds a short-lived provider for that request.
type StoredImageSourceProvider struct {
	store           appports.RuntimeImageSourceStore
	providerFactory appports.ImageProviderFactory

	mu       sync.RWMutex
	lastMeta domain.ImagePoolGenerationMeta
}

var _ appports.ImageProvider = (*StoredImageSourceProvider)(nil)

func NewStoredImageSourceProvider(store appports.RuntimeImageSourceStore, providerFactory appports.ImageProviderFactory) *StoredImageSourceProvider {
	return &StoredImageSourceProvider{
		store:           store,
		providerFactory: providerFactory,
	}
}

func (p *StoredImageSourceProvider) FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error) {
	cfg, err := p.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	provider, err := p.buildProvider(cfg)
	if err != nil {
		return nil, err
	}

	images, err := provider.FetchImages(ctx, count)
	if err != nil {
		return nil, err
	}

	p.recordLastMeta(domain.ImagePoolGenerationMeta{
		SourceConfigVersion: cfg.Version,
		SourceURL:           cfg.URL,
	})
	return images, nil
}

func (p *StoredImageSourceProvider) LastGenerationMeta() domain.ImagePoolGenerationMeta {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastMeta
}

func (p *StoredImageSourceProvider) loadConfig(ctx context.Context) (domain.ImageSourceRuntimeConfig, error) {
	if p == nil || p.store == nil {
		return domain.ImageSourceRuntimeConfig{}, fmt.Errorf("image source store is not configured")
	}

	cfg, found, err := p.store.Load(ctx)
	if err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}
	if !found {
		return domain.ImageSourceRuntimeConfig{}, fmt.Errorf("image source config is missing")
	}
	if err := ValidateImageSourceRuntimeConfig(cfg); err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}
	return cfg, nil
}

func (p *StoredImageSourceProvider) buildProvider(cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	if p == nil || p.providerFactory == nil {
		return nil, fmt.Errorf("image provider factory is not configured")
	}
	return p.providerFactory.BuildRuntimeProvider(cfg)
}

func (p *StoredImageSourceProvider) recordLastMeta(meta domain.ImagePoolGenerationMeta) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastMeta = meta
}

func BuildImageSourceCandidateConfig(current domain.ImageSourceRuntimeConfig, patch domain.ImageSourcePatch) (domain.ImageSourceRuntimeConfig, error) {
	candidate := current

	if patch.URL != nil {
		candidate.URL = strings.TrimSpace(*patch.URL)
	}
	if patch.APIKey != nil {
		candidate.APIKey = strings.TrimSpace(*patch.APIKey)
	}
	if patch.TimeoutSeconds != nil {
		candidate.TimeoutSeconds = *patch.TimeoutSeconds
	}
	if patch.RateLimitPerMinute != nil {
		candidate.RateLimitPerMinute = *patch.RateLimitPerMinute
	}
	if patch.RetryCount != nil {
		candidate.RetryCount = *patch.RetryCount
	}

	candidate = NormalizeImageSourceRuntimeConfig(candidate)
	if err := ValidateImageSourceRuntimeConfig(candidate); err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}

	return candidate, nil
}

func NormalizeImageSourceRuntimeConfig(cfg domain.ImageSourceRuntimeConfig) domain.ImageSourceRuntimeConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg
}

func ValidateImageSourceRuntimeConfig(cfg domain.ImageSourceRuntimeConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("image source url is required")
	}

	parsedURL, err := url.ParseRequestURI(cfg.URL)
	if err != nil || !parsedURL.IsAbs() {
		return fmt.Errorf("image source url must be an absolute URL")
	}

	if cfg.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeoutSeconds must be greater than 0")
	}
	if cfg.RateLimitPerMinute <= 0 {
		return fmt.Errorf("rateLimitPerMinute must be greater than 0")
	}
	if cfg.RetryCount < 0 {
		return fmt.Errorf("retryCount cannot be negative")
	}

	return nil
}

func ImageSourceConfigPublicView(cfg domain.ImageSourceRuntimeConfig) domain.ImageSourceConfigView {
	return domain.ImageSourceConfigView{
		Version:            cfg.Version,
		URL:                cfg.URL,
		APIKeyConfigured:   cfg.APIKey != "",
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
		UpdatedAt:          cfg.UpdatedAt,
	}
}
