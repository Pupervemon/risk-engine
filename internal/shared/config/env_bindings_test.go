package config

import "testing"

func TestRiskCanonicalEnvNamesOverrideConfigFiles(t *testing.T) {
	clearEnv(t,
		"APP_ENV",
		"CONFIG_FILE",
		"RISK_APP_ENV",
		"RISK_CONFIG_FILE",
		"RISK_HTTP_PORT",
		"RISK_NACOS_NAMESPACE",
	)

	t.Setenv("RISK_HTTP_PORT", "12080")
	t.Setenv("RISK_NACOS_NAMESPACE", "canonical-namespace")

	cfg := loadRiskConfigForTest(t)
	if cfg.HTTP.Port != 12080 {
		t.Fatalf("expected canonical HTTP port, got %d", cfg.HTTP.Port)
	}
	if cfg.Nacos.Namespace != "canonical-namespace" {
		t.Fatalf("expected canonical namespace, got %q", cfg.Nacos.Namespace)
	}
}

func TestRiskServerRegistryAliasesAreIgnored(t *testing.T) {
	clearEnv(t,
		"APP_ENV",
		"CONFIG_FILE",
		"RISK_APP_ENV",
		"RISK_CONFIG_FILE",
		"RISK_HTTP_PORT",
		"RISK_SERVER_HTTP_PORT",
		"RISK_NACOS_NAMESPACE",
		"RISK_REGISTRY_NACOS_NAMESPACE",
	)

	t.Setenv("RISK_SERVER_HTTP_PORT", "14080")
	t.Setenv("RISK_REGISTRY_NACOS_NAMESPACE", "compat-namespace")

	cfg := loadRiskConfigForTest(t)
	if cfg.HTTP.Port != 9080 {
		t.Fatalf("expected config file HTTP port when server alias is ignored, got %d", cfg.HTTP.Port)
	}
	if cfg.Nacos.Namespace != "file-namespace" {
		t.Fatalf("expected config file namespace when registry alias is ignored, got %q", cfg.Nacos.Namespace)
	}
}

func TestCaptchaLegacyAliasesStillWork(t *testing.T) {
	clearEnv(t,
		"APP_ENV",
		"CONFIG_FILE",
		"CAPTCHA_APP_ENV",
		"CAPTCHA_CONFIG_FILE",
		"CAPTCHA_TOKEN_SECRET",
		"TOKEN_SECRET",
		"CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_URL",
		"CAPTCHA_EXTERNAL_IMAGE_API_URL",
		"REDIS_ADDR",
		"CAPTCHA_REDIS_ADDR",
	)

	t.Setenv("TOKEN_SECRET", "legacy-secret")
	t.Setenv("REDIS_ADDR", "legacy-redis:6379")
	t.Setenv("CAPTCHA_EXTERNAL_IMAGE_API_URL", "https://legacy.example.com/image")

	cfg := loadCaptchaConfigForTest(t)
	if cfg.Token.Secret != "legacy-secret" {
		t.Fatalf("expected TOKEN_SECRET alias to override config, got %q", cfg.Token.Secret)
	}
	if cfg.Redis.Addr != "legacy-redis:6379" {
		t.Fatalf("expected REDIS_ADDR alias to override config, got %q", cfg.Redis.Addr)
	}
	if cfg.Captcha.ExternalImageAPI.URL != "https://legacy.example.com/image" {
		t.Fatalf("expected legacy external image API alias to override config, got %q", cfg.Captcha.ExternalImageAPI.URL)
	}
}

func TestCaptchaEmptyNacosNamespaceIsAllowedInProd(t *testing.T) {
	cfg := CaptchaConfig{
		HTTP: HTTPConfig{Port: 8091},
		Grpc: GrpcConfig{Port: 9091},
		Redis: RedisConfig{
			Addr:                "redis-internal:6379",
			Password:            "prod-redis-secret",
			DB:                  2,
			PoolSize:            100,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  2,
			WriteTimeoutSeconds: 2,
		},
		Captcha: CaptchaConfigSpec{
			TTLSeconds:      120,
			Width:           320,
			Height:          180,
			GraphSizeMin:    52,
			GraphSizeMax:    64,
			SliderTolerance: 3,
			ImagePool: ImagePoolConfig{
				Enabled:                true,
				PoolSize:               10,
				RefreshIntervalMinutes: 60,
			},
			TrackValidation: TrackValidationConfig{
				Enabled:        true,
				MinPoints:      15,
				MinDurationMs:  800,
				MaxDurationMs:  15000,
				PointTolerance: 20,
			},
			ExternalImageAPI: ExternalImageAPIConfig{
				URL:                "https://example.com/image",
				APIKey:             "api-key",
				TimeoutSeconds:     30,
				RateLimitPerMinute: 50,
				RetryCount:         3,
			},
		},
		Token: TokenConfig{
			TTLSeconds: 300,
			Secret:     "prod-token-secret",
		},
		Nacos: NacosConfig{
			Enable:      true,
			ServerAddr:  "nacos-internal:8848",
			Namespace:   "",
			ServiceName: "captcha-service",
			GroupName:   "DEFAULT_GROUP",
			ClusterName: "prod-cluster",
		},
	}

	if err := validateCaptchaConfigStrict(&cfg, "prod"); err != nil {
		t.Fatalf("expected empty nacos namespace to be allowed in prod, got error: %v", err)
	}
}

func TestRiskEmptyNacosNamespaceIsAllowedInProd(t *testing.T) {
	cfg := RiskConfig{
		HTTP: HTTPConfig{Port: 9080},
		Grpc: GrpcConfig{Port: 9090},
		Redis: RedisConfig{
			Addr:                "redis-internal:6379",
			Password:            "prod-redis-secret",
			DB:                  1,
			PoolSize:            50,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 3,
		},
		Nacos: NacosConfig{
			Enable:      true,
			ServerAddr:  "nacos-internal:8848",
			Namespace:   "",
			ServiceName: "risk-service",
			GroupName:   "DEFAULT_GROUP",
			ClusterName: "prod-cluster",
		},
		RiskRules: RiskRulesConfig{
			Login: LoginRuleConfig{
				MaxFailCount:           3,
				FailCountExpireMinutes: 10,
			},
			IpRateLimit: IPRateLimitConfig{
				Limit:         100,
				WindowSeconds: 1,
			},
			UserRateLimit: UserRateLimitConfig{
				OnlineSelfTest: UserRateLimitRule{
					Limit:         10,
					WindowSeconds: 5,
				},
				JudgeSubmission: UserRateLimitRule{
					Limit:         10,
					WindowSeconds: 10,
				},
			},
		},
	}

	if err := validateRiskConfigStrict(&cfg, "prod"); err != nil {
		t.Fatalf("expected empty nacos namespace to be allowed in prod, got error: %v", err)
	}
}
