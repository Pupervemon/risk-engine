package config

import "time"

// CaptchaConfigSpec 定义验证码相关的运行时配置。
//
// 这一组配置控制验证码生成、校验以及支撑验证码流程的可选外部集成。
// 大部分字段会通过 mapstructure 标签从 YAML 或环境变量加载。
type CaptchaConfigSpec struct {
	// TTLSeconds 表示验证码挑战的存活时间，单位为秒。
	// 小于等于 0 的值会在校验阶段被视为非法，生产环境不应使用。
	TTLSeconds int `mapstructure:"ttl_seconds"`
	// Width 表示生成验证码图片的宽度，单位为像素。
	Width int `mapstructure:"width"`
	// Height 表示生成验证码图片的高度，单位为像素。
	Height int `mapstructure:"height"`
	// GraphSizeMin 表示验证码图片中图形元素数量的最小值。
	GraphSizeMin int `mapstructure:"graph_size_min"`
	// GraphSizeMax 表示验证码图片中图形元素数量的最大值。
	GraphSizeMax int `mapstructure:"graph_size_max"`
	// SliderTolerance 表示滑块答案校验时允许的偏差。
	// 具体含义可能是像素或逻辑单位，取决于挑战类型。
	SliderTolerance int `mapstructure:"slider_tolerance"`
	// ImagePool 配置可选的本地图片池，用于避免每次请求都重新拉取或生成图片。
	ImagePool ImagePoolConfig `mapstructure:"image_pool"`
	// TrackValidation 配置可选的鼠标轨迹校验规则。
	TrackValidation TrackValidationConfig `mapstructure:"track_validation"`
	// ExternalImageAPI 配置上游图片服务，用于验证码图片来源于外部系统的场景。
	ExternalImageAPI ExternalImageAPIConfig `mapstructure:"external_image_api"`
}

// ImagePoolConfig 控制可选的验证码图片池。
//
// 启用后，服务会维护一批预生成图片，并按固定周期刷新，以降低延迟并减轻
// 对外部依赖的压力。
type ImagePoolConfig struct {
	// Enabled 控制图片池是否启用。
	Enabled bool `mapstructure:"enabled"`
	// PoolSize 表示预留在内存或缓存中的图片数量。
	PoolSize int `mapstructure:"pool_size"`
	// RefreshIntervalMinutes 表示图片池补充刷新的间隔，单位为分钟。
	RefreshIntervalMinutes int `mapstructure:"refresh_interval_minutes"`
}

// TrackValidationConfig 控制可选的鼠标轨迹校验。
//
// 这部分用于验证码不仅要校验结果值，还要进一步校验用户交互轨迹的场景。
type TrackValidationConfig struct {
	// Enabled 控制轨迹校验是否启用。
	Enabled bool `mapstructure:"enabled"`
	// MinPoints 表示一条有效轨迹至少需要采样的点数。
	MinPoints int `mapstructure:"min_points"`
	// MinDurationMs 表示允许的最短拖动或交互时长，单位为毫秒。
	MinDurationMs int64 `mapstructure:"min_duration_ms"`
	// MaxDurationMs 表示允许的最长拖动或交互时长，单位为毫秒。
	MaxDurationMs int64 `mapstructure:"max_duration_ms"`
	// PointTolerance 表示轨迹点允许的空间误差。
	PointTolerance int `mapstructure:"point_tolerance"`
}

// ExternalImageAPIConfig 控制上游图片服务。
//
// 这些配置定义验证码图片的来源、调用超时时间，以及服务重试和限流的强度。
type ExternalImageAPIConfig struct {
	// URL 是上游图片接口地址。
	URL string `mapstructure:"url"`
	// APIKey 是上游需要认证时使用的访问密钥。
	APIKey string `mapstructure:"api_key"`
	// TimeoutSeconds 是请求超时时间，单位为秒。
	// 超时后会进入降级或失败处理。
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	// RateLimitPerMinute 表示每分钟最多可发送的上游请求数。
	RateLimitPerMinute int `mapstructure:"rate_limit_per_minute"`
	// RetryCount 表示上游调用失败后的重试次数。
	RetryCount int `mapstructure:"retry_count"`
}

// GetTimeout 将配置的超时时间转换为 duration。
//
// 当配置值小于等于 0 时，回退到 30 秒，确保即使配置不完整，服务也有
// 可预测的网络超时。
func (c *ExternalImageAPIConfig) GetTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// GetRefreshInterval 将配置的刷新间隔转换为 duration。
//
// 当配置值小于等于 0 时，回退到 1 小时，避免在未显式配置时刷新循环过于频繁。
func (c *ImagePoolConfig) GetRefreshInterval() time.Duration {
	if c.RefreshIntervalMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.RefreshIntervalMinutes) * time.Minute
}

// TokenConfig 控制验证码 token 的签发。
//
// 这一部分决定已签发 token 的有效期，以及用于签名和校验的密钥。
type TokenConfig struct {
	// TTLSeconds 是 token 的有效期，单位为秒。
	TTLSeconds int `mapstructure:"ttl_seconds"`
	// Secret 是验证码 token 的签名密钥。
	// 如果希望重启后仍能校验历史 token，这个值必须保持稳定。
	Secret string `mapstructure:"secret"`
}

// CaptchaConfig 是验证码服务完整的运行时配置树。
//
// 它把传输层、存储、验证码行为、token 处理和注册中心集成配置统一到一个对象中。
type CaptchaConfig struct {
	HTTP    HTTPConfig        `mapstructure:"http"`
	Grpc    GrpcConfig        `mapstructure:"grpc"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	Token   TokenConfig       `mapstructure:"token"`
	Nacos   NacosConfig       `mapstructure:"nacos"`
}
