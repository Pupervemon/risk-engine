package config

import "fmt"

// Validate 验证配置是否有效，复用了严格的启动验证逻辑。
func (c *CaptchaConfig) Validate(env string) error {
	return validateCaptchaConfigStrict(c, env)
}

// Print 在控制台打印脱敏后的启动配置摘要。
func (c *CaptchaConfig) Print() {
	fmt.Println("\n=========== Captcha Service Config ===========")
	fmt.Printf("HTTP port: %d\n", c.HTTP.Port)
	fmt.Printf("gRPC port: %d\n", c.Grpc.Port)

	fmt.Println("----------------------------------------------")
	fmt.Printf("Redis addr: %s\n", c.Redis.Addr)
	fmt.Printf("Redis DB: %d\n", c.Redis.DB)
	fmt.Printf("Redis password: %s\n", maskedConfigValue(c.Redis.Password))

	fmt.Println("----------------------------------------------")
	fmt.Printf("Token secret: %s\n", maskedConfigValue(c.Token.Secret))
	fmt.Printf("Captcha TTL: %ds\n", c.Captcha.TTLSeconds)
	fmt.Printf("Token TTL: %ds\n", c.Token.TTLSeconds)

	fmt.Println("----------------------------------------------")
	fmt.Printf("Nacos: %s\n", boolLabel(c.Nacos.Enable))
	if c.Nacos.Enable {
		fmt.Printf("Nacos server: %s\n", c.Nacos.ServerAddr)
		fmt.Printf("Nacos namespace: %s\n", displayNamespace(c.Nacos.Namespace))
		fmt.Printf("Nacos service: %s\n", c.Nacos.ServiceName)
	}

	fmt.Println("----------------------------------------------")
	fmt.Printf("Image pool: %s\n", boolLabel(c.Captcha.ImagePool.Enabled))
	if c.Captcha.ImagePool.Enabled {
		fmt.Printf("Image pool size: %d\n", c.Captcha.ImagePool.PoolSize)
		fmt.Printf("Image refresh interval: %d min\n", c.Captcha.ImagePool.RefreshIntervalMinutes)
		fmt.Printf("External image API: %s\n", displayExternalAPIURL(c.Captcha.ExternalImageAPI.URL))
	}

	fmt.Println("----------------------------------------------")
	fmt.Printf("Track validation: %s\n", boolLabel(c.Captcha.TrackValidation.Enabled))
	if c.Captcha.TrackValidation.Enabled {
		fmt.Printf("Track min points: %d\n", c.Captcha.TrackValidation.MinPoints)
		fmt.Printf("Track duration range: %d-%dms\n",
			c.Captcha.TrackValidation.MinDurationMs,
			c.Captcha.TrackValidation.MaxDurationMs)
	}

	fmt.Println("==============================================\n")
}

// displayExternalAPIURL 处理外部 API URL 的显示，为空时返回 <unset>。
func displayExternalAPIURL(url string) string {
	if url == "" {
		return "<unset>"
	}
	return url
}

// toggleStr 是 boolLabel 的别名，用于向后兼容。
func toggleStr(enabled bool) string {
	return boolLabel(enabled)
}
