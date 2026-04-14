package config

import "testing"

func TestResolveAppEnvPriority(t *testing.T) {
	t.Setenv("RISK_APP_ENV", "svc")
	t.Setenv("APP_ENV", "global")

	if got := resolveAppEnv("RISK", "explicit"); got != "explicit" {
		t.Fatalf("expected explicit env override, got %q", got)
	}
	if got := resolveAppEnv("RISK", ""); got != "svc" {
		t.Fatalf("expected service env override, got %q", got)
	}

	t.Setenv("RISK_APP_ENV", "")
	if got := resolveAppEnv("RISK", ""); got != "global" {
		t.Fatalf("expected global env fallback, got %q", got)
	}

	t.Setenv("APP_ENV", "")
	if got := resolveAppEnv("RISK", ""); got != "prod" {
		t.Fatalf("expected default prod env, got %q", got)
	}
}

func TestResolveConfigFilePriority(t *testing.T) {
	t.Setenv("RISK_CONFIG_FILE", "service.yaml")
	t.Setenv("CONFIG_FILE", "global.yaml")

	if got := resolveConfigFile("RISK", "explicit.yaml"); got != "explicit.yaml" {
		t.Fatalf("expected explicit config override, got %q", got)
	}
	if got := resolveConfigFile("RISK", ""); got != "service.yaml" {
		t.Fatalf("expected service config override, got %q", got)
	}

	t.Setenv("RISK_CONFIG_FILE", "")
	if got := resolveConfigFile("RISK", ""); got != "global.yaml" {
		t.Fatalf("expected global config fallback, got %q", got)
	}

	t.Setenv("CONFIG_FILE", "")
	if got := resolveConfigFile("RISK", ""); got != "" {
		t.Fatalf("expected empty config file fallback, got %q", got)
	}
}
