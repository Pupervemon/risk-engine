package config

// LoadCaptchaConfig 是旧版的公共入口点，现在内部使用统一的加载器。
// configPath: 配置文件路径
func LoadCaptchaConfig(configPath string) (*CaptchaConfig, error) {
	return LoadCaptchaConfigWithOptions(LoadOptions{ConfigPath: configPath})
}
