package ports

import (
	"context"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
)

type CheckCommand struct {
	ReqID       string
	Scene       domain.Scene
	IP          string
	UserID      string
	PhoneNumber string
	DeviceID    string
	Timestamp   int64
}

type CheckDecision struct {
	Action domain.Action
	Reason string
}

type ReportEventCommand struct {
	ReqID     string
	Scene     domain.Scene
	IP        string
	UserID    string
	IsSuccess bool
	ExtraInfo string
}

type ReportEventResult struct {
	Received bool
}

type AddBlacklistCommand struct {
	Type     domain.BlacklistType
	Value    string
	Reason   string
	ExpireAt int64
}

type AddBlacklistResult struct {
	Success bool
}

type UserActionCommand struct {
	ReqID     string
	UserID    string
	Payload   string
	Answer    string
	Timestamp int64
}

type UserActionResult struct {
	Accepted bool
	Reason   string
}

type RiskIPListQuery struct {
	Offset int
	Limit  int
	Search string
}

type RiskIPEventsQuery struct {
	Offset int
	Limit  int
}

type RiskIPListResponse struct {
	Items   []domain.RiskIPSummary `json:"items"`
	Offset  int                    `json:"offset"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	HasMore bool                   `json:"has_more"`
}

type RiskIPEventsResponse struct {
	IP      string               `json:"ip"`
	Items   []domain.RiskIPEvent `json:"items"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
	Total   int64                `json:"total"`
	HasMore bool                 `json:"has_more"`
}

type RiskCheckUseCase interface {
	Check(ctx context.Context, cmd CheckCommand) (CheckDecision, error)
}

type RiskEventUseCase interface {
	ReportEvent(ctx context.Context, cmd ReportEventCommand) (ReportEventResult, error)
}

type BlacklistUseCase interface {
	AddBlacklist(ctx context.Context, cmd AddBlacklistCommand) (AddBlacklistResult, error)
}

type UserThrottleUseCase interface {
	OnlineSelfTest(ctx context.Context, cmd UserActionCommand) (UserActionResult, error)
	JudgeSubmission(ctx context.Context, cmd UserActionCommand) (UserActionResult, error)
}

type RiskInsightQuery interface {
	ListRiskIPs(ctx context.Context, query RiskIPListQuery) (*RiskIPListResponse, error)
	GetRiskIP(ctx context.Context, ip string) (*domain.RiskIPDetail, error)
	ListRiskIPEvents(ctx context.Context, ip string, query RiskIPEventsQuery) (*RiskIPEventsResponse, error)
}
