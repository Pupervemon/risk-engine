package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	sharedconfig "github.com/Pupervemon/risk-engine/internal/shared/config"
	"go.uber.org/zap"
)

var ErrImagePoolDisabled = errors.New("captcha image pool is disabled")

// ImageSourceRefreshError indicates that the candidate config validated successfully
// but the image pool refresh step failed against the upstream source.
type ImageSourceRefreshError struct {
	Err error
}

func (e *ImageSourceRefreshError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ImageSourceRefreshError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ImageSourcePersistenceError indicates that the runtime config could not be persisted.
type ImageSourcePersistenceError struct {
	Err error
}

func (e *ImageSourcePersistenceError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ImageSourcePersistenceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ImageSourcePatch represents a partial update to the runtime upstream image source config.
type ImageSourcePatch struct {
	URL                *string
	APIKey             *string
	TimeoutSeconds     *int
	RateLimitPerMinute *int
	RetryCount         *int
}

// ImageSourceRuntimeConfig is the mutable runtime config used by the active image provider.
type ImageSourceRuntimeConfig struct {
	URL                string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceConfigView is the sanitized public view of the runtime image source config.
type ImageSourceConfigView struct {
	URL                string `json:"url"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
}

// ImageSourceStatus describes the current runtime image source state.
type ImageSourceStatus struct {
	Enabled             bool                  `json:"enabled"`
	Version             int64                 `json:"version"`
	Config              ImageSourceConfigView `json:"config"`
	UpdatedAt           string                `json:"updatedAt,omitempty"`
	LastValidatedAt     string                `json:"lastValidatedAt,omitempty"`
	LastValidationError string                `json:"lastValidationError,omitempty"`
	LastRefreshedAt     string                `json:"lastRefreshedAt,omitempty"`
	LastRefreshError    string                `json:"lastRefreshError,omitempty"`
	PoolSize            int                   `json:"poolSize"`
	PoolImageCount      int64                 `json:"poolImageCount"`
}

// ImageSourceValidationResult is returned by the validate endpoint.
type ImageSourceValidationResult struct {
	Config      ImageSourceConfigView `json:"config"`
	ValidatedAt string                `json:"validatedAt,omitempty"`
}

// RuntimeImageSourceManager owns the active upstream image source config and provider.
type RuntimeImageSourceManager struct {
	logger *zap.Logger
	width  int
	height int

	mu                  sync.RWMutex
	config              ImageSourceRuntimeConfig
	provider            ImageProvider
	version             int64
	updatedAt           time.Time
	lastValidatedAt     time.Time
	lastValidationError string
	lastRefreshedAt     time.Time
	lastRefreshError    string
}

// NewRuntimeImageSourceManager creates a runtime-switchable image provider manager.
func NewRuntimeImageSourceManager(initial ImageSourceRuntimeConfig, logger *zap.Logger, width, height int) (*RuntimeImageSourceManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	initial = normalizeImageSourceRuntimeConfig(initial)
	provider, err := buildRuntimeImageProvider(initial, logger, width, height)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &RuntimeImageSourceManager{
		logger:    logger,
		width:     width,
		height:    height,
		config:    initial,
		provider:  provider,
		version:   1,
		updatedAt: now,
	}, nil
}

// FetchImages delegates to the active provider.
func (m *RuntimeImageSourceManager) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	provider := m.activeProvider()
	if provider == nil {
		return nil, fmt.Errorf("image provider is not configured")
	}

	return provider.FetchImages(ctx, count)
}

// BuildCandidateConfig merges a patch onto the current config and validates the result.
func (m *RuntimeImageSourceManager) BuildCandidateConfig(patch ImageSourcePatch) (ImageSourceRuntimeConfig, error) {
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
		return ImageSourceRuntimeConfig{}, err
	}

	return candidate, nil
}

// ValidateConfig verifies that a candidate config can fetch and process at least one image.
func (m *RuntimeImageSourceManager) ValidateConfig(ctx context.Context, candidate ImageSourceRuntimeConfig) (ImageProvider, error) {
	provider, err := buildRuntimeImageProvider(candidate, m.logger, m.width, m.height)
	if err == nil {
		var images []ImageMeta
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

// ApplyConfig makes a validated provider the active runtime provider.
func (m *RuntimeImageSourceManager) ApplyConfig(candidate ImageSourceRuntimeConfig, provider ImageProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = candidate
	m.provider = provider
	m.version++
	m.updatedAt = time.Now()
}

// RestoreConfig loads a persisted config into a fresh manager without bumping the version.
func (m *RuntimeImageSourceManager) RestoreConfig(candidate ImageSourceRuntimeConfig, provider ImageProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = candidate
	m.provider = provider
	m.updatedAt = time.Now()
}

// RecordRefreshResult stores the result of the last explicit or scheduled image pool refresh.
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

// ValidationResult returns the public validation response for a candidate config.
func (m *RuntimeImageSourceManager) ValidationResult(candidate ImageSourceRuntimeConfig) ImageSourceValidationResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ImageSourceValidationResult{
		Config:      candidate.publicView(),
		ValidatedAt: formatOptionalTime(m.lastValidatedAt),
	}
}

// Status returns the public runtime status snapshot.
func (m *RuntimeImageSourceManager) Status(poolSize int, poolImageCount int64) ImageSourceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ImageSourceStatus{
		Enabled:             true,
		Version:             m.version,
		Config:              m.config.publicView(),
		UpdatedAt:           formatOptionalTime(m.updatedAt),
		LastValidatedAt:     formatOptionalTime(m.lastValidatedAt),
		LastValidationError: m.lastValidationError,
		LastRefreshedAt:     formatOptionalTime(m.lastRefreshedAt),
		LastRefreshError:    m.lastRefreshError,
		PoolSize:            poolSize,
		PoolImageCount:      poolImageCount,
	}
}

func (m *RuntimeImageSourceManager) activeProvider() ImageProvider {
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

func buildRuntimeImageProvider(cfg ImageSourceRuntimeConfig, logger *zap.Logger, width, height int) (ImageProvider, error) {
	if err := validateImageSourceRuntimeConfig(cfg); err != nil {
		return nil, err
	}

	return NewExternalImageFetcher(cfg.fetcherConfig(), logger, width, height), nil
}

func normalizeImageSourceRuntimeConfig(cfg ImageSourceRuntimeConfig) ImageSourceRuntimeConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg
}

func validateImageSourceRuntimeConfig(cfg ImageSourceRuntimeConfig) error {
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

func (cfg ImageSourceRuntimeConfig) fetcherConfig() ExternalImageAPIConfig {
	return ExternalImageAPIConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		Timeout:            time.Duration(cfg.TimeoutSeconds) * time.Second,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func (cfg ImageSourceRuntimeConfig) publicView() ImageSourceConfigView {
	return ImageSourceConfigView{
		URL:                cfg.URL,
		APIKeyConfigured:   cfg.APIKey != "",
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func imageSourceRuntimeConfigFromShared(cfg sharedconfig.ExternalImageAPIConfig) ImageSourceRuntimeConfig {
	return ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
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
