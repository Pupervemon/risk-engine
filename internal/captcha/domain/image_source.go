package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// ImageMeta is a normalized background image stored in the captcha image pool.
type ImageMeta struct {
	ID   string
	Data []byte
	URL  string
}

// ImagePoolSnapshot describes the currently active image-pool generation.
type ImagePoolSnapshot struct {
	ImageCount          int64
	ActiveGeneration    string
	GenerationCount     int64
	SourceConfigVersion int64
	SourceURL           string
	RefreshedAt         string
}

// ImagePoolGenerationMeta records which image-source config produced a pool generation.
type ImagePoolGenerationMeta struct {
	Generation          string
	SourceConfigVersion int64
	SourceURL           string
	ImageCount          int64
	CreatedAt           string
}

// ImageSourcePatch is a partial runtime image-source update.
type ImageSourcePatch struct {
	URL                *string
	APIKey             *string
	TimeoutSeconds     *int
	RateLimitPerMinute *int
	RetryCount         *int
}

// ImageSourceRuntimeConfig is the runtime image-source config stored in Redis.
type ImageSourceRuntimeConfig struct {
	Version            int64
	URL                string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
	UpdatedAt          string
}

func BuildImageSourceCandidateConfig(current ImageSourceRuntimeConfig, patch ImageSourcePatch) (ImageSourceRuntimeConfig, error) {
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

	candidate = candidate.Normalized()
	if err := candidate.Validate(); err != nil {
		return ImageSourceRuntimeConfig{}, err
	}

	return candidate, nil
}

func (cfg ImageSourceRuntimeConfig) Normalized() ImageSourceRuntimeConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg
}

func (cfg ImageSourceRuntimeConfig) Validate() error {
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

func (cfg ImageSourceRuntimeConfig) PublicView() ImageSourceConfigView {
	return ImageSourceConfigView{
		Version:            cfg.Version,
		URL:                cfg.URL,
		APIKeyConfigured:   cfg.APIKey != "",
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
		UpdatedAt:          cfg.UpdatedAt,
	}
}

// ImageSourceConfigView is the safe public view of an image-source config.
type ImageSourceConfigView struct {
	Version            int64
	URL                string
	APIKeyConfigured   bool
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
	UpdatedAt          string
}

// ImageSourceActivePoolView describes the image pool currently serving captcha requests.
type ImageSourceActivePoolView struct {
	SourceConfigVersion int64
	SourceURL           string
	ImageCount          int64
	RefreshedAt         string
}

// ImageSourceSyncStatus compares the current config with the active pool.
type ImageSourceSyncStatus struct {
	PoolSyncedWithConfig bool
	Message              string
}

// ImageSourceRuntimeStatus keeps operational timestamps and errors in Redis.
type ImageSourceRuntimeStatus struct {
	LastValidatedAt     string
	LastValidationError string
	LastRefreshedAt     string
	LastRefreshError    string
}

// ImageSourceStatus is the application status returned by image-source admin APIs.
type ImageSourceStatus struct {
	Enabled             bool
	Config              ImageSourceConfigView
	ActivePool          ImageSourceActivePoolView
	Sync                ImageSourceSyncStatus
	UpdatedAt           string
	LastValidatedAt     string
	LastValidationError string
	LastRefreshedAt     string
	LastRefreshError    string
	PoolSize            int
	PoolImageCount      int64
	ActiveGeneration    string
	GenerationCount     int64
}

// ImageSourceValidationResult is returned after checking the current source config.
type ImageSourceValidationResult struct {
	Config      ImageSourceConfigView
	ValidatedAt string
}
