package domain

import "errors"

var (
	ErrInvalidRiskIP      = errors.New("invalid risk ip")
	ErrRiskIPNotFound     = errors.New("risk ip not found")
	ErrUnsupportedScene   = errors.New("unsupported scene")
	ErrLoginIdentityEmpty = errors.New("login event identity empty")
	ErrUserIDEmpty        = errors.New("user id empty")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
	ErrRequestEmpty       = errors.New("request empty")
	ErrInvalidBlacklist   = errors.New("invalid blacklist")
)
