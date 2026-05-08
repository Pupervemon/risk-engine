package ports

import (
	"context"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
)

type BlacklistRepository interface {
	Get(ctx context.Context, entryType domain.BlacklistType, value string) (domain.BlacklistEntry, bool, error)
	Save(ctx context.Context, entry domain.BlacklistEntry) error
}

type RateLimiter interface {
	IncrementIP(ctx context.Context, ip string, rule domain.RateLimitRule) (domain.RateLimitResult, error)
	IncrementUser(ctx context.Context, userID string, scope string, rule domain.RateLimitRule) (domain.RateLimitResult, error)
}

type LoginFailureCounter interface {
	GetByUserID(ctx context.Context, userID string) (int64, error)
	GetByIP(ctx context.Context, ip string) (int64, error)
	RecordFailure(ctx context.Context, target domain.LoginFailureTarget, expireSeconds int) (int64, error)
	Clear(ctx context.Context, target domain.LoginFailureTarget) error
}

type RiskInsightRepository interface {
	RecordCheck(ctx context.Context, insight domain.RiskCheckInsight) error
	RecordReportEvent(ctx context.Context, insight domain.RiskReportInsight) error
	RecordBlacklist(ctx context.Context, insight domain.RiskBlacklistInsight) error
	ListRiskIPs(ctx context.Context, query RiskIPListQuery, loginFailThreshold int64) (*RiskIPListResponse, error)
	GetRiskIP(ctx context.Context, ip string, loginFailThreshold int64) (*domain.RiskIPDetail, error)
	ListRiskIPEvents(ctx context.Context, ip string, query RiskIPEventsQuery) (*RiskIPEventsResponse, error)
}
