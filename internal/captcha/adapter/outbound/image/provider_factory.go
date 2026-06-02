package imageadapter

import (
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"go.uber.org/zap"
)

// ExternalImageProviderFactory builds image providers backed by the external
// image adapter.
type ExternalImageProviderFactory struct {
	logger *zap.Logger
	width  int
	height int
}

var _ appports.ImageProviderFactory = (*ExternalImageProviderFactory)(nil)

func NewExternalImageProviderFactory(logger *zap.Logger, width, height int) *ExternalImageProviderFactory {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ExternalImageProviderFactory{
		logger: logger,
		width:  width,
		height: height,
	}
}

func (f *ExternalImageProviderFactory) BuildRuntimeProvider(cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error) {
	return NewExternalImageFetcher(externalImageAPIConfigFromRuntime(cfg), f.logger, f.width, f.height), nil
}

func externalImageAPIConfigFromRuntime(cfg domain.ImageSourceRuntimeConfig) ExternalImageAPIConfig {
	return ExternalImageAPIConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		Timeout:            runtimeTimeout(cfg.TimeoutSeconds),
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

func runtimeTimeout(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
