package config

import "fmt"

// validateRiskConfigStrict runs strict startup validation for the risk service.
func validateRiskConfigStrict(cfg *RiskConfig, env string) error {
	if cfg == nil {
		return fmt.Errorf("risk config cannot be nil")
	}

	if err := validatePositiveInt("HTTP port", cfg.HTTP.Port); err != nil {
		return err
	}
	if err := validatePositiveInt("gRPC port", cfg.Grpc.Port); err != nil {
		return err
	}
	if err := validateNonNegativeInt("grpc.request_timeout_seconds", cfg.Grpc.RequestTimeoutSeconds); err != nil {
		return err
	}
	if err := validateRedisConfig(cfg.Redis); err != nil {
		return err
	}
	if err := validateNacosConfig(cfg.Nacos, env); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.login.max_fail_count", cfg.RiskRules.Login.MaxFailCount); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.login.fail_count_expire_minutes", cfg.RiskRules.Login.FailCountExpireMinutes); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.ip_rate_limit.limit", cfg.RiskRules.IpRateLimit.Limit); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.ip_rate_limit.window_seconds", cfg.RiskRules.IpRateLimit.WindowSeconds); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.user_rate_limit.online_self_test.limit", cfg.RiskRules.UserRateLimit.OnlineSelfTest.Limit); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.user_rate_limit.online_self_test.window_seconds", cfg.RiskRules.UserRateLimit.OnlineSelfTest.WindowSeconds); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.user_rate_limit.judge_submission.limit", cfg.RiskRules.UserRateLimit.JudgeSubmission.Limit); err != nil {
		return err
	}
	if err := validatePositiveInt("risk_rules.user_rate_limit.judge_submission.window_seconds", cfg.RiskRules.UserRateLimit.JudgeSubmission.WindowSeconds); err != nil {
		return err
	}

	if env == "prod" && cfg.Redis.Password == "" {
		return fmt.Errorf("[security] redis password is required in prod")
	}

	return nil
}

// validateCaptchaConfigStrict runs strict startup validation for the captcha service.
func validateCaptchaConfigStrict(cfg *CaptchaConfig, env string) error {
	if cfg == nil {
		return fmt.Errorf("captcha config cannot be nil")
	}

	if err := validatePositiveInt("HTTP port", cfg.HTTP.Port); err != nil {
		return err
	}
	if err := validatePositiveInt("gRPC port", cfg.Grpc.Port); err != nil {
		return err
	}
	if err := validateNonNegativeInt("grpc.request_timeout_seconds", cfg.Grpc.RequestTimeoutSeconds); err != nil {
		return err
	}
	if err := validateRedisConfig(cfg.Redis); err != nil {
		return err
	}
	if err := validateNacosConfig(cfg.Nacos, env); err != nil {
		return err
	}
	if err := validatePositiveInt("captcha.ttl_seconds", cfg.Captcha.TTLSeconds); err != nil {
		return err
	}
	if err := validatePositiveInt("captcha.width", cfg.Captcha.Width); err != nil {
		return err
	}
	if err := validatePositiveInt("captcha.height", cfg.Captcha.Height); err != nil {
		return err
	}
	if err := validatePositiveInt("captcha.graph_size_min", cfg.Captcha.GraphSizeMin); err != nil {
		return err
	}
	if cfg.Captcha.GraphSizeMax < cfg.Captcha.GraphSizeMin {
		return fmt.Errorf("captcha.graph_size_max cannot be smaller than graph_size_min")
	}
	if err := validatePositiveInt("captcha.slider_tolerance", cfg.Captcha.SliderTolerance); err != nil {
		return err
	}
	if err := validatePositiveInt("token.ttl_seconds", cfg.Token.TTLSeconds); err != nil {
		return err
	}

	if cfg.Captcha.ImagePool.Enabled {
		if err := validatePositiveInt("captcha.image_pool.pool_size", cfg.Captcha.ImagePool.PoolSize); err != nil {
			return err
		}
		if err := validatePositiveInt("captcha.image_pool.refresh_interval_minutes", cfg.Captcha.ImagePool.RefreshIntervalMinutes); err != nil {
			return err
		}
		if cfg.Captcha.ExternalImageAPI.URL == "" {
			return fmt.Errorf("captcha.external_image_api.url is required when the image pool is enabled")
		}
		if err := validatePositiveInt("captcha.external_image_api.timeout_seconds", cfg.Captcha.ExternalImageAPI.TimeoutSeconds); err != nil {
			return err
		}
		if err := validatePositiveInt("captcha.external_image_api.rate_limit_per_minute", cfg.Captcha.ExternalImageAPI.RateLimitPerMinute); err != nil {
			return err
		}
		if err := validateNonNegativeInt("captcha.external_image_api.retry_count", cfg.Captcha.ExternalImageAPI.RetryCount); err != nil {
			return err
		}
	}

	if cfg.Captcha.TrackValidation.Enabled {
		if err := validatePositiveInt("captcha.track_validation.min_points", cfg.Captcha.TrackValidation.MinPoints); err != nil {
			return err
		}
		if err := validatePositiveInt64("captcha.track_validation.min_duration_ms", cfg.Captcha.TrackValidation.MinDurationMs); err != nil {
			return err
		}
		if cfg.Captcha.TrackValidation.MaxDurationMs < cfg.Captcha.TrackValidation.MinDurationMs {
			return fmt.Errorf("captcha.track_validation.max_duration_ms cannot be smaller than min_duration_ms")
		}
		if err := validatePositiveInt("captcha.track_validation.point_tolerance", cfg.Captcha.TrackValidation.PointTolerance); err != nil {
			return err
		}
	}

	if env == "prod" {
		if cfg.Redis.Password == "" {
			return fmt.Errorf("[security] redis password is required in prod")
		}
		if isPlaceholderSecret(cfg.Token.Secret) {
			return fmt.Errorf("[security] token secret is missing or still set to a placeholder in prod")
		}
	}

	return nil
}
