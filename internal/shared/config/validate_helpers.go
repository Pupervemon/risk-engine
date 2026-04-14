package config

import (
	"fmt"
	"net"
	"strings"
)

// validateRedisConfig validates Redis connectivity settings.
func validateRedisConfig(cfg RedisConfig) error {
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("redis addr is required")
	}
	if cfg.DB < 0 {
		return fmt.Errorf("redis DB cannot be negative")
	}
	if cfg.PoolSize <= 0 {
		return fmt.Errorf("redis pool_size must be greater than 0")
	}
	if cfg.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("redis dial_timeout_seconds must be greater than 0")
	}
	if cfg.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("redis read_timeout_seconds must be greater than 0")
	}
	if cfg.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("redis write_timeout_seconds must be greater than 0")
	}
	return nil
}

// validateNacosConfig validates Nacos registry settings.
func validateNacosConfig(cfg NacosConfig, env string) error {
	if !cfg.Enable {
		return nil
	}

	if strings.TrimSpace(cfg.ServerAddr) == "" {
		return fmt.Errorf("nacos server_addr is required when nacos is enabled")
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return fmt.Errorf("nacos service_name is required when nacos is enabled")
	}
	if strings.TrimSpace(cfg.GroupName) == "" {
		return fmt.Errorf("nacos group_name is required when nacos is enabled")
	}
	if strings.TrimSpace(cfg.ClusterName) == "" {
		return fmt.Errorf("nacos cluster_name is required when nacos is enabled")
	}
	if env == "prod" && strings.TrimSpace(cfg.Namespace) == "" {
		return fmt.Errorf("[security] nacos namespace is required in prod when nacos is enabled")
	}
	if cfg.RegisterIP != "" {
		parsed := net.ParseIP(cfg.RegisterIP)
		if parsed == nil || parsed.To4() == nil {
			return fmt.Errorf("invalid nacos register_ip: %s", cfg.RegisterIP)
		}
	}
	return nil
}

// validatePositiveInt validates that the value is greater than zero.
func validatePositiveInt(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

// validatePositiveInt64 validates that the int64 value is greater than zero.
func validatePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

// validateNonNegativeInt validates that the value is not negative.
func validateNonNegativeInt(name string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s cannot be negative", name)
	}
	return nil
}

// isPlaceholderSecret reports whether the configured secret is still a placeholder.
func isPlaceholderSecret(secret string) bool {
	normalized := strings.TrimSpace(secret)
	switch normalized {
	case "", "CHANGE_ME", "YOUR_TOKEN_SECRET", "dev_default_secret_key":
		return true
	default:
		return false
	}
}
