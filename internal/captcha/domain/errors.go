package domain

import "errors"

var (
	ErrCaptchaNotFound            = errors.New("captcha not found")
	ErrImagePoolDisabled          = errors.New("captcha image pool is disabled")
	ErrImagePoolRefreshInProgress = errors.New("captcha image pool refresh is already in progress")
)

// ImageSourceRefreshError means a valid image source config failed while
// refreshing the image pool or fetching upstream images.
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

// ImageSourcePersistenceError means an accepted image source config could not
// be persisted.
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
