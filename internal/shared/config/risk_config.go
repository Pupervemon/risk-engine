package config

import (
	"fmt"
	"strconv"
)

// RiskConfig 是风控服务的完整运行时配置树。
type RiskConfig struct {
	// HTTP 是 HTTP 服务配置
	HTTP HTTPConfig `mapstructure:"http"`
	// Grpc 是 gRPC 服务配置
	Grpc GrpcConfig `mapstructure:"grpc"`
	// Redis 是 Redis 连接配置
	Redis RedisConfig `mapstructure:"redis"`
	// Nacos 是 Nacos 注册中心配置
	Nacos NacosConfig `mapstructure:"nacos"`
	// RiskRules 是风控规则配置
	RiskRules RiskRulesConfig `mapstructure:"risk_rules"`
}

// RiskRulesConfig 汇总了所有风控规则设置。
type RiskRulesConfig struct {
	// Login 登录规则配置
	Login LoginRuleConfig `mapstructure:"login"`
	// IpRateLimit IP 频率限制配置
	IpRateLimit IPRateLimitConfig `mapstructure:"ip_rate_limit"`
	// UserRateLimit 用户维度频率限制配置
	UserRateLimit UserRateLimitConfig `mapstructure:"user_rate_limit"`
}

// LoginRuleConfig 登录保护规则
type LoginRuleConfig struct {
	// MaxFailCount 最大失败次数
	MaxFailCount int `mapstructure:"max_fail_count"`
	// FailCountExpireMinutes 失败记录有效期（分钟）
	FailCountExpireMinutes int `mapstructure:"fail_count_expire_minutes"`
}

// IPRateLimitConfig IP 维度频率限制
type IPRateLimitConfig struct {
	// Limit 限制次数
	Limit int `mapstructure:"limit"`
	// WindowSeconds 时间窗口（秒）
	WindowSeconds int `mapstructure:"window_seconds"`
}

// UserRateLimitConfig 用户维度频率限制汇总
type UserRateLimitConfig struct {
	// OnlineSelfTest 在线自测频率限制
	OnlineSelfTest UserRateLimitRule `mapstructure:"online_self_test"`
	// JudgeSubmission 判题提交频率限制
	JudgeSubmission UserRateLimitRule `mapstructure:"judge_submission"`
}

// UserRateLimitRule 通用的用户频率限制规则
type UserRateLimitRule struct {
	// Limit 限制次数
	Limit int `mapstructure:"limit"`
	// WindowSeconds 时间窗口（秒）
	WindowSeconds int `mapstructure:"window_seconds"`
}

// LoadRiskConfig 是旧版的公共入口点，委托给统一加载器执行。
func LoadRiskConfig(configPath string) (*RiskConfig, error) {
	return LoadRiskConfigWithOptions(LoadOptions{ConfigPath: configPath})
}

// LoadRiskConfigWithOptions 根据显式的配置和环境变量覆盖加载风控服务配置。
func LoadRiskConfigWithOptions(options LoadOptions) (*RiskConfig, error) {
	v, env, err := newServiceViper("risk", "RISK", options, RiskConfig{})
	if err != nil {
		return nil, err
	}

	fmt.Printf("[RiskConfig] loading environment: %s\n", env)
	fmt.Printf("[RiskConfig] using config file: %s\n", v.ConfigFileUsed())

	var cfg RiskConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal risk config: %w", err)
	}

	if cfg.Nacos.Enable {
		if cfg.Nacos.Metadata == nil {
			cfg.Nacos.Metadata = make(map[string]string)
		}
		// Keep the existing metadata key for compatibility until the registry contract is unified.
		cfg.Nacos.Metadata["gRPC_port"] = strconv.Itoa(cfg.Grpc.Port)
	}
	if err := validateRiskConfigStrict(&cfg, env); err != nil {
		return nil, err
	}

	cfg.Print()

	return &cfg, nil
}
