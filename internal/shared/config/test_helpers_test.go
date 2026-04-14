package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadRiskConfigForTest(t *testing.T) RiskConfig {
	t.Helper()

	v, env, err := newServiceViper("risk", "RISK", LoadOptions{
		ConfigPath: testConfigDir(t),
		Env:        "dev",
	}, RiskConfig{})
	if err != nil {
		t.Fatalf("load risk viper: %v", err)
	}
	if env != "dev" {
		t.Fatalf("expected dev env, got %q", env)
	}

	var cfg RiskConfig
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal risk config: %v", err)
	}
	return cfg
}

func loadCaptchaConfigForTest(t *testing.T) CaptchaConfig {
	t.Helper()

	v, env, err := newServiceViper("captcha", "CAPTCHA", LoadOptions{
		ConfigPath: testConfigDir(t),
		Env:        "dev",
	}, CaptchaConfig{})
	if err != nil {
		t.Fatalf("load captcha viper: %v", err)
	}
	if env != "dev" {
		t.Fatalf("expected dev env, got %q", env)
	}

	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal captcha config: %v", err)
	}
	return cfg
}

func testConfigDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Join(wd, "testdata")
}

func clearEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		t.Setenv(name, "")
	}
}
