package config

import "strings"

// maskedConfigValue redacts a sensitive config value for logging.
func maskedConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unset>"
	}
	return "******"
}

// boolLabel converts a boolean into a stable enabled/disabled label.
func boolLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// displayNamespace keeps the namespace output stable, falling back to public.
func displayNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "public"
	}
	return namespace
}
