package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Grpc      GrpcConfig       `mapstructure:"grpc"`
	Redis     RedisConfig      `mapstructure:"redis"`
	RiskRules *RiskRulesConfig `mapstructure:"risk_rules"`
}

type GrpcConfig struct {
	Port int `mapstructure:"port"`
}

type RedisConfig struct {
	Addr                string `mapstructure:"addr"`
	Password            string `mapstructure:"password"`
	DB                  int    `mapstructure:"db"`
	PoolSize            int    `mapstructure:"pool_size"`
	DialTimeoutSeconds  int    `mapstructure:"dial_timeout_seconds"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
}

type RiskRulesConfig struct {
	Login         LoginRuleConfig     `mapstructure:"login"`
	IpRateLimit   RateLimitConfig     `mapstructure:"ip_rate_limit"`
	UserRateLimit UserRateLimitConfig `mapstructure:"user_rate_limit"`
}

type UserRateLimitConfig struct {
	OnlineSelfTest  RateLimitConfig `mapstructure:"online_self_test"`
	JudgeSubmission RateLimitConfig `mapstructure:"judge_submission"`
}

type LoginRuleConfig struct {
	MaxFailCount           int64 `mapstructure:"max_fail_count"`
	FailCountExpireMinutes int   `mapstructure:"fail_count_expire_minutes"`
}

type RateLimitConfig struct {
	Limit         int64 `mapstructure:"limit"`
	WindowSeconds int   `mapstructure:"window_seconds"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.AddConfigPath(path)
	v.SetConfigName("risk")
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
