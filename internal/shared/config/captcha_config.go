package config

import "time"

// CaptchaConfigSpec defines captcha-specific runtime settings.
type CaptchaConfigSpec struct {
	TTLSeconds       int                    `mapstructure:"ttl_seconds"`
	Width            int                    `mapstructure:"width"`
	Height           int                    `mapstructure:"height"`
	GraphSizeMin     int                    `mapstructure:"graph_size_min"`
	GraphSizeMax     int                    `mapstructure:"graph_size_max"`
	SliderTolerance  int                    `mapstructure:"slider_tolerance"`
	ImagePool        ImagePoolConfig        `mapstructure:"image_pool"`
	TrackValidation  TrackValidationConfig  `mapstructure:"track_validation"`
	ExternalImageAPI ExternalImageAPIConfig `mapstructure:"external_image_api"`
}

// ImagePoolConfig controls the optional captcha image pool.
type ImagePoolConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	PoolSize               int  `mapstructure:"pool_size"`
	RefreshIntervalMinutes int  `mapstructure:"refresh_interval_minutes"`
}

// TrackValidationConfig controls optional mouse-track validation.
type TrackValidationConfig struct {
	Enabled        bool  `mapstructure:"enabled"`
	MinPoints      int   `mapstructure:"min_points"`
	MinDurationMs  int64 `mapstructure:"min_duration_ms"`
	MaxDurationMs  int64 `mapstructure:"max_duration_ms"`
	PointTolerance int   `mapstructure:"point_tolerance"`
}

// ExternalImageAPIConfig controls the upstream image provider.
type ExternalImageAPIConfig struct {
	URL                string `mapstructure:"url"`
	APIKey             string `mapstructure:"api_key"`
	TimeoutSeconds     int    `mapstructure:"timeout_seconds"`
	RateLimitPerMinute int    `mapstructure:"rate_limit_per_minute"`
	RetryCount         int    `mapstructure:"retry_count"`
}

// GetTimeout converts the configured timeout to a duration.
func (c *ExternalImageAPIConfig) GetTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// GetRefreshInterval converts the configured refresh interval to a duration.
func (c *ImagePoolConfig) GetRefreshInterval() time.Duration {
	if c.RefreshIntervalMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.RefreshIntervalMinutes) * time.Minute
}

// TokenConfig controls captcha token issuance.
type TokenConfig struct {
	TTLSeconds int    `mapstructure:"ttl_seconds"`
	Secret     string `mapstructure:"secret"`
}

// CaptchaConfig is the full runtime config tree for the captcha service.
type CaptchaConfig struct {
	HTTP    HTTPConfig        `mapstructure:"http"`
	Grpc    GrpcConfig        `mapstructure:"grpc"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	Token   TokenConfig       `mapstructure:"token"`
	Nacos   NacosConfig       `mapstructure:"nacos"`
}
