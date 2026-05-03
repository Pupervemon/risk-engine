package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"go.uber.org/zap"
)

// RuntimeImageSourceManager keeps the active runtime image-source config and
// provider in memory.
type RuntimeImageSourceManager struct {
	logger          *zap.Logger
	width           int
	height          int
	providerFactory appports.ImageProviderFactory

	mu                  sync.RWMutex
	config              domain.ImageSourceRuntimeConfig
	provider            appports.ImageProvider
	version             int64
	updatedAt           time.Time
	lastValidatedAt     time.Time
	lastValidationError string
	lastRefreshedAt     time.Time
	lastRefreshError    string
}

var (
	_ appports.RuntimeImageSourceManager = (*RuntimeImageSourceManager)(nil)
	_ appports.ImageProvider             = (*RuntimeImageSourceManager)(nil)
)

func NewRuntimeImageSourceManager(initial domain.ImageSourceRuntimeConfig, logger *zap.Logger, width, height int, providerFactory appports.ImageProviderFactory) (*RuntimeImageSourceManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if providerFactory == nil {
		return nil, fmt.Errorf("image provider factory is not configured")
	}

	initial = normalizeImageSourceRuntimeConfig(initial)
	if err := validateImageSourceRuntimeConfig(initial); err != nil {
		return nil, err
	}
	provider, err := providerFactory.BuildRuntimeProvider(initial)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &RuntimeImageSourceManager{
		logger:          logger,
		width:           width,
		height:          height,
		providerFactory: providerFactory,
		config:          initial,
		provider:        provider,
		version:         1,
		updatedAt:       now,
	}, nil
}

func (m *RuntimeImageSourceManager) FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error) {
	provider := m.activeProvider()
	if provider == nil {
		return nil, fmt.Errorf("image provider is not configured")
	}

	return provider.FetchImages(ctx, count)
}

func (m *RuntimeImageSourceManager) BuildCandidateConfig(patch domain.ImageSourcePatch) (domain.ImageSourceRuntimeConfig, error) {
	m.mu.RLock()
	candidate := m.config
	m.mu.RUnlock()

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

	candidate = normalizeImageSourceRuntimeConfig(candidate)
	if err := validateImageSourceRuntimeConfig(candidate); err != nil {
		return domain.ImageSourceRuntimeConfig{}, err
	}

	return candidate, nil
}

func (m *RuntimeImageSourceManager) ValidateConfig(ctx context.Context, candidate domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	provider, err := m.BuildProvider(candidate)
	if err == nil {
		var images []domain.ImageMeta
		images, err = provider.FetchImages(ctx, 1)
		if err == nil && len(images) == 0 {
			err = fmt.Errorf("validation fetched zero images")
		}
	}

	m.recordValidationResult(err)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func (m *RuntimeImageSourceManager) ApplyConfig(candidate domain.ImageSourceRuntimeConfig, provider appports.ImageProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = candidate
	m.provider = provider
	m.version++
	m.updatedAt = time.Now()
}

func (m *RuntimeImageSourceManager) RestoreConfig(candidate domain.ImageSourceRuntimeConfig, provider appports.ImageProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = candidate
	m.provider = provider
	m.updatedAt = time.Now()
}

func (m *RuntimeImageSourceManager) RecordRefreshResult(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastRefreshedAt = time.Now()
	if err != nil {
		m.lastRefreshError = err.Error()
		return
	}

	m.lastRefreshError = ""
}

func (m *RuntimeImageSourceManager) ValidationResult(candidate domain.ImageSourceRuntimeConfig) domain.ImageSourceValidationResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return domain.ImageSourceValidationResult{
		Config:      imageSourceConfigPublicView(candidate),
		ValidatedAt: formatOptionalTime(m.lastValidatedAt),
	}
}

func (m *RuntimeImageSourceManager) Status(poolSize int, poolSnapshot domain.ImagePoolSnapshot) domain.ImageSourceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return domain.ImageSourceStatus{
		Enabled:             true,
		Version:             m.version,
		Config:              imageSourceConfigPublicView(m.config),
		UpdatedAt:           formatOptionalTime(m.updatedAt),
		LastValidatedAt:     formatOptionalTime(m.lastValidatedAt),
		LastValidationError: m.lastValidationError,
		LastRefreshedAt:     formatOptionalTime(m.lastRefreshedAt),
		LastRefreshError:    m.lastRefreshError,
		PoolSize:            poolSize,
		PoolImageCount:      poolSnapshot.ImageCount,
		ActiveGeneration:    poolSnapshot.ActiveGeneration,
		GenerationCount:     poolSnapshot.GenerationCount,
	}
}

func (m *RuntimeImageSourceManager) BuildProvider(cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	if m == nil || m.providerFactory == nil {
		return nil, fmt.Errorf("image provider factory is not configured")
	}
	if err := validateImageSourceRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	return m.providerFactory.BuildRuntimeProvider(cfg)
}

func (m *RuntimeImageSourceManager) activeProvider() appports.ImageProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

func (m *RuntimeImageSourceManager) recordValidationResult(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastValidatedAt = time.Now()
	if err != nil {
		m.lastValidationError = err.Error()
		return
	}

	m.lastValidationError = ""
}

func normalizeImageSourceRuntimeConfig(cfg domain.ImageSourceRuntimeConfig) domain.ImageSourceRuntimeConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg
}

func validateImageSourceRuntimeConfig(cfg domain.ImageSourceRuntimeConfig) error {
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

func imageSourceConfigPublicView(cfg domain.ImageSourceRuntimeConfig) domain.ImageSourceConfigView {
	return domain.ImageSourceConfigView{
		URL:                cfg.URL,
		APIKeyConfigured:   cfg.APIKey != "",
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
