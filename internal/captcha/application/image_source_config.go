package application

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

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
