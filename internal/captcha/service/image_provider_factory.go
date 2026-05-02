package service

import "go.uber.org/zap"

// RuntimeImageProviderFactory builds providers for runtime image-source config.
type RuntimeImageProviderFactory interface {
	BuildRuntimeProvider(cfg ImageSourceRuntimeConfig) (ImageProvider, error)
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

func (f *ExternalImageProviderFactory) BuildRuntimeProvider(cfg ImageSourceRuntimeConfig) (ImageProvider, error) {
	if err := validateImageSourceRuntimeConfig(cfg); err != nil {
		return nil, err
	}

	return NewExternalImageFetcher(cfg.fetcherConfig(), f.logger, f.width, f.height), nil
}

func (f *ExternalImageProviderFactory) BuildImagePoolProvider(cfg ExternalImageAPIConfig) ImageProvider {
	return CustomImageFetcher(cfg, f.logger, f.width, f.height)
}
