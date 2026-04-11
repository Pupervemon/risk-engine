package config

import (
	"fmt"
	"net"
	"strings"
)

// validateRedisConfig 验证 Redis 连接配置的完整性。
func validateRedisConfig(cfg RedisConfig) error {
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("Redis地址不能为空")
	}
	if cfg.DB < 0 {
		return fmt.Errorf("Redis DB 不能小于 0")
	}
	if cfg.PoolSize <= 0 {
		return fmt.Errorf("Redis 连接池大小必须大于 0")
	}
	if cfg.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("Redis DialTimeoutSeconds 必须大于 0")
	}
	if cfg.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("Redis ReadTimeoutSeconds 必须大于 0")
	}
	if cfg.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("Redis WriteTimeoutSeconds 必须大于 0")
	}
	return nil
}

// validateNacosConfig 验证 Nacos 注册中心配置的正确性。
func validateNacosConfig(cfg NacosConfig, env string) error {
	if !cfg.Enable {
		return nil
	}

	if strings.TrimSpace(cfg.ServerAddr) == "" {
		return fmt.Errorf("启用 Nacos 时 server_addr 不能为空")
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return fmt.Errorf("启用 Nacos 时 service_name 不能为空")
	}
	if strings.TrimSpace(cfg.GroupName) == "" {
		return fmt.Errorf("启用 Nacos 时 group_name 不能为空")
	}
	if strings.TrimSpace(cfg.ClusterName) == "" {
		return fmt.Errorf("启用 Nacos 时 cluster_name 不能为空")
	}
	if env == "prod" && strings.TrimSpace(cfg.Namespace) == "" {
		return fmt.Errorf("[安全阻断] 生产环境启用 Nacos 时 namespace 不能为空")
	}
	if cfg.RegisterIP != "" {
		parsed := net.ParseIP(cfg.RegisterIP)
		if parsed == nil || parsed.To4() == nil {
			return fmt.Errorf("Nacos register_ip 无效: %s", cfg.RegisterIP)
		}
	}
	return nil
}

// validatePositiveInt 验证整数是否为正数。
func validatePositiveInt(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s 必须大于 0", name)
	}
	return nil
}

// validatePositiveInt64 验证 int64 是否为正数。
func validatePositiveInt64(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s 必须大于 0", name)
	}
	return nil
}

// validateNonNegativeInt 验证整数是否为非负数。
func validateNonNegativeInt(name string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s 不能小于 0", name)
	}
	return nil
}

// isPlaceholderSecret 检查密钥是否为占位符，用于安全校验。
func isPlaceholderSecret(secret string) bool {
	normalized := strings.TrimSpace(secret)
	switch normalized {
	case "", "CHANGE_ME", "YOUR_TOKEN_SECRET", "dev_default_secret_key":
		return true
	default:
		return false
	}
}
