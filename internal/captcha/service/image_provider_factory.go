package service

import (
	imageadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/image"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"go.uber.org/zap"
)

type ExternalImageAPIConfig = imageadapter.ExternalImageAPIConfig

// RuntimeImageProviderFactory builds providers for runtime image-source config.
type RuntimeImageProviderFactory interface {
	BuildRuntimeProvider(cfg domain.ImageSourceRuntimeConfig) (appports.ImageProvider, error)
}

// ExternalImageProviderFactory keeps provider construction behind a boundary
// while the fetcher implementation is still in the service package.
type ExternalImageProviderFactory struct {
	logger *zap.Logger
	width  int
	height int
}

var _ RuntimeImageProviderFactory = (*ExternalImageProviderFactory)(nil)

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
	if err := validateImageSourceRuntimeConfig(cfg); err != nil {
		return nil, err
	}

	return imageadapter.NewExternalImageFetcher(imageSourceFetcherConfig(cfg), f.logger, f.width, f.height), nil
}

func (f *ExternalImageProviderFactory) BuildImagePoolProvider(cfg ExternalImageAPIConfig) appports.ImageProvider {
	return imageadapter.CustomImageFetcher(cfg, f.logger, f.width, f.height)
}
