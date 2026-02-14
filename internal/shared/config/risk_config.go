package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// UserRateLimitRule 用户级别限流规则
type UserRateLimitRule struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

// UserRateLimitConfig 用户级别限流配置
type UserRateLimitConfig struct {
	OnlineSelfTest  UserRateLimitRule `mapstructure:"online_self_test"`
	JudgeSubmission UserRateLimitRule `mapstructure:"judge_submission"`
}

// LoginRuleConfig 登录风控规则配置
type LoginRuleConfig struct {
	MaxFailCount           int `mapstructure:"max_fail_count"`
	FailCountExpireMinutes int `mapstructure:"fail_count_expire_minutes"`
}

// IPRateLimitConfig IP级别限流配置
type IPRateLimitConfig struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

// RiskRulesConfig 风控规则配置
type RiskRulesConfig struct {
	Login         LoginRuleConfig     `mapstructure:"login"`
	IpRateLimit   IPRateLimitConfig   `mapstructure:"ip_rate_limit"`
	UserRateLimit UserRateLimitConfig `mapstructure:"user_rate_limit"`
}

// RiskConfig 全局配置结构
type RiskConfig struct {
	HTTP      HTTPConfig      `mapstructure:"http"`
	Grpc      GrpcConfig      `mapstructure:"grpc"`
	Redis     RedisConfig     `mapstructure:"redis"`
	RiskRules RiskRulesConfig `mapstructure:"risk_rules"`
	Nacos     NacosConfig     `mapstructure:"nacos"`
}

// LoadConfig 加载配置文件
func LoadConfig(configPath string) (*RiskConfig, error) {
	v := viper.New()
	v.SetConfigName("risk")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg RiskConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}
