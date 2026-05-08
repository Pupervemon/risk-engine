package application

import "github.com/Pupervemon/risk-engine/internal/risk/domain"

type RiskOptions struct {
	Login         LoginOptions
	IPRateLimit   domain.RateLimitRule
	UserRateLimit UserRateLimitOptions
}

type LoginOptions struct {
	MaxFailCount           int
	FailCountExpireMinutes int
}

type UserRateLimitOptions struct {
	OnlineSelfTest  domain.RateLimitRule
	JudgeSubmission domain.RateLimitRule
}
