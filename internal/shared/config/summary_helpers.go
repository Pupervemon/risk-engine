package config

import "strings"

// maskedConfigValue 对敏感配置值进行掩码处理，用于日志显示。
func maskedConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unset>"
	}
	return "******"
}

// boolLabel 将布尔值转换为可读的 enabled/disabled 标签。
func boolLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// displayNamespace 处理 Nacos 命名空间的显示，为空时返回 public。
func displayNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "public"
	}
	return namespace
}
