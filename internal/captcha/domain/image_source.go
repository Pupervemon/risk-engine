package domain

// ImageMeta 包含验证码图片池使用的标准化图片数据。
type ImageMeta struct {
	ID   string
	Data []byte
	URL  string
}

// ImagePoolSnapshot 描述当前活跃的图片池代次。
type ImagePoolSnapshot struct {
	ImageCount       int64
	ActiveGeneration string
	GenerationCount  int64
}

// ImageSourcePatch 表示运行时图片源的部分更新。
type ImageSourcePatch struct {
	URL                *string
	APIKey             *string
	TimeoutSeconds     *int
	RateLimitPerMinute *int
	RetryCount         *int
}

// ImageSourceRuntimeConfig 是生效中的运行时图片源配置。
type ImageSourceRuntimeConfig struct {
	URL                string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceConfigView 是图片源配置的安全对外视图。
type ImageSourceConfigView struct {
	URL                string
	APIKeyConfigured   bool
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceStatus 是图片源管理接口使用的应用快照。
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

// ImageSourceValidationResult 是图片源校验的返回结果。
type ImageSourceValidationResult struct {
	Config      ImageSourceConfigView
	ValidatedAt string
}
