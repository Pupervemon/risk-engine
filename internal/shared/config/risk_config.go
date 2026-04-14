package config

import (
	"fmt"
	"strconv"
)

// RiskConfig is the full runtime config tree for the risk service.
type RiskConfig struct {
	HTTP      HTTPConfig      `mapstructure:"http"`
	Grpc      GrpcConfig      `mapstructure:"grpc"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Nacos     NacosConfig     `mapstructure:"nacos"`
	RiskRules RiskRulesConfig `mapstructure:"risk_rules"`
}

// RiskRulesConfig groups all risk rule settings.
type RiskRulesConfig struct {
	Login         LoginRuleConfig     `mapstructure:"login"`
	IpRateLimit   IPRateLimitConfig   `mapstructure:"ip_rate_limit"`
	UserRateLimit UserRateLimitConfig `mapstructure:"user_rate_limit"`
}

// LoginRuleConfig defines login protection rules.
type LoginRuleConfig struct {
	MaxFailCount           int `mapstructure:"max_fail_count"`
	FailCountExpireMinutes int `mapstructure:"fail_count_expire_minutes"`
}

// IPRateLimitConfig defines IP-based rate limiting.
type IPRateLimitConfig struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

// UserRateLimitConfig groups user-scoped rate limits.
type UserRateLimitConfig struct {
	OnlineSelfTest  UserRateLimitRule `mapstructure:"online_self_test"`
	JudgeSubmission UserRateLimitRule `mapstructure:"judge_submission"`
}

// UserRateLimitRule defines a generic user-scoped rate limit.
type UserRateLimitRule struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

// LoadRiskConfig preserves the legacy entrypoint while delegating to the unified loader.
func LoadRiskConfig(configPath string) (*RiskConfig, error) {
	return LoadRiskConfigWithOptions(LoadOptions{ConfigPath: configPath})
}

// LoadRiskConfigWithOptions loads Risk config with explicit config and env overrides.
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
