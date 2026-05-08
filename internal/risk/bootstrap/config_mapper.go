package bootstrap

import (
	"github.com/Pupervemon/risk-engine/internal/risk/application"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
)

func RiskOptionsFromSharedConfig(cfg *config.RiskRulesConfig) application.RiskOptions {
	if cfg == nil {
		return application.RiskOptions{}
	}

	return application.RiskOptions{
		Login: application.LoginOptions{
			MaxFailCount:           cfg.Login.MaxFailCount,
			FailCountExpireMinutes: cfg.Login.FailCountExpireMinutes,
		},
		IPRateLimit: domain.RateLimitRule{
			Limit:         cfg.IpRateLimit.Limit,
			WindowSeconds: cfg.IpRateLimit.WindowSeconds,
		},
		UserRateLimit: application.UserRateLimitOptions{
			OnlineSelfTest: domain.RateLimitRule{
				Limit:         cfg.UserRateLimit.OnlineSelfTest.Limit,
				WindowSeconds: cfg.UserRateLimit.OnlineSelfTest.WindowSeconds,
			},
			JudgeSubmission: domain.RateLimitRule{
				Limit:         cfg.UserRateLimit.JudgeSubmission.Limit,
				WindowSeconds: cfg.UserRateLimit.JudgeSubmission.WindowSeconds,
			},
		},
	}
}
