package config

import (
	"fmt"
	"strconv"
)

// LoadCaptchaConfigWithOptions loads Captcha config with explicit config and env overrides.
func LoadCaptchaConfigWithOptions(options LoadOptions) (*CaptchaConfig, error) {
	v, env, err := newServiceViper("captcha", "CAPTCHA", options, CaptchaConfig{})
	if err != nil {
		return nil, err
	}

	fmt.Printf("[CaptchaConfig] loading environment: %s\n", env)
	fmt.Printf("[CaptchaConfig] using config file: %s\n", v.ConfigFileUsed())

	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal captcha config: %w", err)
	}

	if cfg.Nacos.Enable {
		if cfg.Nacos.Metadata == nil {
			cfg.Nacos.Metadata = make(map[string]string)
		}
		// Keep the existing metadata key for compatibility until the registry contract is unified.
		cfg.Nacos.Metadata["gRPC_port"] = strconv.Itoa(cfg.Grpc.Port)
	}

	if err := validateCaptchaConfigStrict(&cfg, env); err != nil {
		return nil, err
	}

	cfg.Print()

	return &cfg, nil
}
