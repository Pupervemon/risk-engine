package domain

// ImageMeta contains normalized image data used by the captcha image pool.
type ImageMeta struct {
	ID   string
	Data []byte
	URL  string
}

// ImagePoolSnapshot describes the active image-pool generation.
type ImagePoolSnapshot struct {
	ImageCount       int64
	ActiveGeneration string
	GenerationCount  int64
}

// ImageSourcePatch represents a partial runtime image source update.
type ImageSourcePatch struct {
	URL                *string
	APIKey             *string
	TimeoutSeconds     *int
	RateLimitPerMinute *int
	RetryCount         *int
}

// ImageSourceRuntimeConfig is the effective runtime image source config.
type ImageSourceRuntimeConfig struct {
	URL                string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceConfigView is a safe external view of an image source config.
type ImageSourceConfigView struct {
	URL                string
	APIKeyConfigured   bool
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceStatus is the application snapshot used by image-source admin APIs.
type ImageSourceStatus struct {
	Enabled             bool
	Version             int64
	Config              ImageSourceConfigView
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

// ImageSourceValidationResult is returned by image-source validation.
type ImageSourceValidationResult struct {
	Config      ImageSourceConfigView
	ValidatedAt string
}
