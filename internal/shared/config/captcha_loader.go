package config

import (
	"fmt"
	"strconv"
)

// LoadCaptchaConfigWithOptions 根据显式的配置和环境变量覆盖加载验证码服务配置。
func LoadCaptchaConfigWithOptions(options LoadOptions) (*CaptchaConfig, error) {
	v, env, err := newServiceViper("captcha", "CAPTCHA", options, CaptchaConfig{})
	if err != nil {
		return nil, err
	}

	fmt.Printf("[CaptchaConfig] 正在加载环境: %s\n", env)
	fmt.Printf("[CaptchaConfig] 使用配置文件: %s\n", v.ConfigFileUsed())

	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置结构失败: %w", err)
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
