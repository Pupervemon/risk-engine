package service

import (
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

var ErrImagePoolRefreshInProgress = domain.ErrImagePoolRefreshInProgress

// ImageMeta contains normalized image data stored in the captcha image pool.
type ImageMeta = domain.ImageMeta

// ImageProvider fetches images from an upstream source.
type ImageProvider = appports.ImageProvider
