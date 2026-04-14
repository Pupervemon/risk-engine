package config

// LoadCaptchaConfig preserves the legacy public entrypoint while delegating to the unified loader.
func LoadCaptchaConfig(configPath string) (*CaptchaConfig, error) {
	return LoadCaptchaConfigWithOptions(LoadOptions{ConfigPath: configPath})
}
