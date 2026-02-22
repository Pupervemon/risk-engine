package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// CaptchaConfigSpec 业务规则配置
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

// ImagePoolConfig 图片池配置
type ImagePoolConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	PoolSize               int  `mapstructure:"pool_size"`
	RefreshIntervalMinutes int  `mapstructure:"refresh_interval_minutes"`
}

// TrackValidationConfig 轨迹校验配置
type TrackValidationConfig struct {
	Enabled        bool  `mapstructure:"enabled"`
	MinPoints      int   `mapstructure:"min_points"`
	MinDurationMs  int64 `mapstructure:"min_duration_ms"`
	MaxDurationMs  int64 `mapstructure:"max_duration_ms"`
	PointTolerance int   `mapstructure:"point_tolerance"`
}

// ExternalImageAPIConfig 外部图片API配置
type ExternalImageAPIConfig struct {
	URL                string `mapstructure:"url"`
	APIKey             string `mapstructure:"api_key"`
	TimeoutSeconds     int    `mapstructure:"timeout_seconds"`
	RateLimitPerMinute int    `mapstructure:"rate_limit_per_minute"`
	RetryCount         int    `mapstructure:"retry_count"`
}

// GetTimeout 获取超时时间
func (c *ExternalImageAPIConfig) GetTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// GetRefreshInterval 获取刷新间隔
func (c *ImagePoolConfig) GetRefreshInterval() time.Duration {
	if c.RefreshIntervalMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.RefreshIntervalMinutes) * time.Minute
}

// TokenConfig Token配置
type TokenConfig struct {
	TTLSeconds int    `mapstructure:"ttl_seconds"`
	Secret     string `mapstructure:"secret"`
}

// CaptchaConfig 全局配置结构
type CaptchaConfig struct {
	HTTP    HTTPConfig        `mapstructure:"http"`
	Grpc    GrpcConfig        `mapstructure:"grpc"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	Token   TokenConfig       `mapstructure:"token"`
	Nacos   NacosConfig       `mapstructure:"nacos"`
}

// LoadCaptchaConfig 加载配置文件（支持多环境）
// 通过环境变量 APP_ENV 或 ENV 指定环境：dev | prod
// 默认为 prod 环境
func LoadCaptchaConfig(configPath string) (*CaptchaConfig, error) {
	// [1] 加载 .env
	_ = godotenv.Load()

	// [2] 确定环境
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENV")
	}
	if env == "" {
		env = "prod" // 默认生产环境
	}

	fmt.Printf("[CaptchaConfig] 正在加载环境: %s\n", env)

	v := viper.New()

	// [3] 开启自动环境变量映射 (核心优化)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// [4] 配置文件定位
	// 文件名格式: captcha.dev.yaml 或 captcha.prod.yaml
	configName := fmt.Sprintf("captcha.%s", env)
	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	// 添加搜索路径
	v.AddConfigPath(configPath)
	v.AddConfigPath("./configs")
	v.AddConfigPath("../configs")
	v.AddConfigPath(".")

	// [5] 读取配置
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s.yaml 失败: %w", configName, err)
	}
	fmt.Printf("[CaptchaConfig] 使用配置文件: %s\n", v.ConfigFileUsed())

	// [6] 解析到结构体
	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置结构失败: %w", err)
	}

	// [7] 校验配置
	if err := cfg.Validate(env); err != nil {
		return nil, err
	}

	// [8] 打印摘要
	cfg.Print()

	return &cfg, nil
}

// Validate 验证配置是否合法
func (c *CaptchaConfig) Validate(env string) error {
	if c.Redis.Addr == "" {
		return fmt.Errorf("Redis地址不能为空")
	}
	if c.HTTP.Port <= 0 {
		return fmt.Errorf("HTTP端口无效")
	}

	// 生产环境严格检查
	if env == "prod" {
		if c.Redis.Password == "" {
			return fmt.Errorf("[安全阻断] 生产环境 Redis 密码不能为空")
		}
		if c.Token.Secret == "" || c.Token.Secret == "CHANGE_ME" {
			return fmt.Errorf("[安全阻断] 生产环境 Token 密钥未设置或使用默认值")
		}
		// 生产环境如果启用外部图片API，必须配置URL
		if c.Captcha.ImagePool.Enabled && c.Captcha.ExternalImageAPI.URL == "" {
			return fmt.Errorf("[配置错误] 启用图片池但未配置外部图片API URL")
		}
	}
	return nil
}

// Print 打印配置信息（脱敏）
func (c *CaptchaConfig) Print() {
	fmt.Println("\n=========== Captcha 服务配置 ===========")
	fmt.Printf("HTTP端口: %d\n", c.HTTP.Port)
	fmt.Printf("gRPC端口: %d\n", c.Grpc.Port)

	fmt.Println("-------------------------------------")
	fmt.Printf("Redis地址: %s\n", c.Redis.Addr)
	fmt.Printf("Redis DB : %d\n", c.Redis.DB)
	// 简单脱敏
	passMask := "<无>"
	if len(c.Redis.Password) > 0 {
		passMask = "******"
	}
	fmt.Printf("Redis密码: %s\n", passMask)

	fmt.Println("-------------------------------------")
	// Token 密钥脱敏
	secretMask := "<未设置>"
	if len(c.Token.Secret) > 0 {
		secretMask = "******"
	}
	fmt.Printf("Token密钥: %s\n", secretMask)
	fmt.Printf("验证码TTL: %d秒\n", c.Captcha.TTLSeconds)
	fmt.Printf("Token TTL: %d秒\n", c.Token.TTLSeconds)

	fmt.Println("-------------------------------------")
	if c.Nacos.Enable {
		ns := c.Nacos.Namespace
		if ns == "" {
			ns = "public (默认)"
		}
		fmt.Printf("Nacos: 启用\n")
		fmt.Printf("地址 : %s\n", c.Nacos.ServerAddr)
		fmt.Printf("空间 : %s\n", ns)
		fmt.Printf("服务名: %s\n", c.Nacos.ServiceName)
	} else {
		fmt.Printf("Nacos: [禁用]\n")
	}

	fmt.Println("-------------------------------------")
	fmt.Printf("图片池: %s\n", toggleStr(c.Captcha.ImagePool.Enabled))
	if c.Captcha.ImagePool.Enabled {
		fmt.Printf("  池大小: %d张\n", c.Captcha.ImagePool.PoolSize)
		fmt.Printf("  刷新间隔: %d分钟\n", c.Captcha.ImagePool.RefreshIntervalMinutes)
		apiURL := c.Captcha.ExternalImageAPI.URL
		if apiURL == "" {
			apiURL = "<未配置-将使用Mock图片>"
		}
		fmt.Printf("  图片API: %s\n", apiURL)
	}

	fmt.Println("-------------------------------------")
	fmt.Printf("轨迹校验: %s\n", toggleStr(c.Captcha.TrackValidation.Enabled))
	if c.Captcha.TrackValidation.Enabled {
		fmt.Printf("  最小点数: %d\n", c.Captcha.TrackValidation.MinPoints)
		fmt.Printf("  时长范围: %d-%dms\n",
			c.Captcha.TrackValidation.MinDurationMs,
			c.Captcha.TrackValidation.MaxDurationMs)
	}

	fmt.Println("=====================================")
	fmt.Println()
}

// toggleStr 开关状态字符串
func toggleStr(enabled bool) string {
	if enabled {
		return "启用"
	}
	return "[禁用]"
}
