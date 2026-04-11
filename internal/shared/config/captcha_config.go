package config

import "time"

// CaptchaConfigSpec 定义了验证码业务相关的运行时设置。
type CaptchaConfigSpec struct {
	// TTLSeconds 验证码有效期（秒）
	TTLSeconds int `mapstructure:"ttl_seconds"`
	// Width 验证码图片宽度
	Width int `mapstructure:"width"`
	// Height 验证码图片高度
	Height int `mapstructure:"height"`
	// GraphSizeMin 滑块/图形最小尺寸
	GraphSizeMin int `mapstructure:"graph_size_min"`
	// GraphSizeMax 滑块/图形最大尺寸
	GraphSizeMax int `mapstructure:"graph_size_max"`
	// SliderTolerance 滑块拼图容错像素值
	SliderTolerance int `mapstructure:"slider_tolerance"`
	// ImagePool 图片池配置
	ImagePool ImagePoolConfig `mapstructure:"image_pool"`
	// TrackValidation 轨迹校验配置
	TrackValidation TrackValidationConfig `mapstructure:"track_validation"`
	// ExternalImageAPI 外部图片 API 配置
	ExternalImageAPI ExternalImageAPIConfig `mapstructure:"external_image_api"`
}

// ImagePoolConfig 控制可选的验证码图片池。
type ImagePoolConfig struct {
	// Enabled 是否启用图片池
	Enabled bool `mapstructure:"enabled"`
	// PoolSize 图片池大小
	PoolSize int `mapstructure:"pool_size"`
	// RefreshIntervalMinutes 图片刷新间隔（分钟）
	RefreshIntervalMinutes int `mapstructure:"refresh_interval_minutes"`
}

// TrackValidationConfig 控制可选的鼠标轨迹验证。
type TrackValidationConfig struct {
	// Enabled 是否启用轨迹验证
	Enabled bool `mapstructure:"enabled"`
	// MinPoints 最小轨迹点数
	MinPoints int `mapstructure:"min_points"`
	// MinDurationMs 最小操作耗时（毫秒）
	MinDurationMs int64 `mapstructure:"min_duration_ms"`
	// MaxDurationMs 最大操作耗时（毫秒）
	MaxDurationMs int64 `mapstructure:"max_duration_ms"`
	// PointTolerance 轨迹点抖动容忍度
	PointTolerance int `mapstructure:"point_tolerance"`
}

// ExternalImageAPIConfig 控制上游图片提供者。
type ExternalImageAPIConfig struct {
	// URL 外部图片 API 地址
	URL string `mapstructure:"url"`
	// APIKey 访问外部 API 的密钥
	APIKey string `mapstructure:"api_key"`
	// TimeoutSeconds API 调用超时时间（秒）
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	// RateLimitPerMinute API 调用频率限制（每分钟）
	RateLimitPerMinute int `mapstructure:"rate_limit_per_minute"`
	// RetryCount 失败重试次数
	RetryCount int `mapstructure:"retry_count"`
}

// GetTimeout 将配置的超时秒数转换为 time.Duration。
func (c *ExternalImageAPIConfig) GetTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// GetRefreshInterval 将配置的刷新间隔（分钟）转换为 time.Duration。
func (c *ImagePoolConfig) GetRefreshInterval() time.Duration {
	if c.RefreshIntervalMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.RefreshIntervalMinutes) * time.Minute
}

// TokenConfig 控制验证码令牌的签发。
type TokenConfig struct {
	// TTLSeconds 令牌有效期（秒）
	TTLSeconds int `mapstructure:"ttl_seconds"`
	// Secret 令牌签发密钥
	Secret string `mapstructure:"secret"`
}

// CaptchaConfig 是验证码服务的完整配置树。
type CaptchaConfig struct {
	// HTTP 是 HTTP 服务配置
	HTTP HTTPConfig `mapstructure:"http"`
	// Grpc 是 gRPC 服务配置
	Grpc GrpcConfig `mapstructure:"grpc"`
	// Redis 是 Redis 连接配置
	Redis RedisConfig `mapstructure:"redis"`
	// Captcha 是验证码业务配置
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	// Token 是令牌配置
	Token TokenConfig `mapstructure:"token"`
	// Nacos 是 Nacos 注册中心配置
	Nacos NacosConfig `mapstructure:"nacos"`
}
