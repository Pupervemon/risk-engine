package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	"go.uber.org/zap"
)

const (
	ScopeOnlineSelfTest  = "online_self_test"
	ScopeJudgeSubmission = "judge_submission"
)

type RiskUseCase struct {
	Blacklist    ports.BlacklistRepository
	RateLimiter  ports.RateLimiter
	LoginFailure ports.LoginFailureCounter
	Insights     ports.RiskInsightRepository
	Options      RiskOptions
	Logger       *zap.Logger
}

func NewRiskUseCase(
	blacklist ports.BlacklistRepository,
	rateLimiter ports.RateLimiter,
	loginFailure ports.LoginFailureCounter,
	insights ports.RiskInsightRepository,
	options RiskOptions,
	logger *zap.Logger,
) *RiskUseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RiskUseCase{
		Blacklist:    blacklist,
		RateLimiter:  rateLimiter,
		LoginFailure: loginFailure,
		Insights:     insights,
		Options:      options,
		Logger:       logger,
	}
}

func (u *RiskUseCase) Check(ctx context.Context, cmd ports.CheckCommand) (ports.CheckDecision, error) {
	finish := func(decision ports.CheckDecision) (ports.CheckDecision, error) {
		if cmd.IP != "" && u.Insights != nil {
			if err := u.Insights.RecordCheck(ctx, domain.RiskCheckInsight{
				ReqID:  cmd.ReqID,
				Scene:  cmd.Scene,
				IP:     cmd.IP,
				UserID: cmd.UserID,
				Action: decision.Action,
				Reason: decision.Reason,
			}); err != nil {
				u.Logger.Warn("failed to record risk ip check insight", zap.Error(err), zap.String("ip", cmd.IP))
			}
		}
		return decision, nil
	}

	if cmd.IP == "" {
		return finish(ports.CheckDecision{Action: domain.ActionReject, Reason: "INVALID_REQUEST_IP_EMPTY"})
	}

	if hit, reason := u.checkBlacklist(ctx, cmd); hit {
		u.Logger.Warn("blacklist hit",
			zap.String("ip", cmd.IP),
			zap.String("userId", cmd.UserID),
			zap.String("reason", reason))
		return finish(ports.CheckDecision{Action: domain.ActionReject, Reason: reason})
	}

	if u.checkIPRateLimit(ctx, cmd.IP) {
		return finish(ports.CheckDecision{Action: domain.ActionReject, Reason: "IP_RATE_LIMIT_EXCEEDED"})
	}

	if cmd.Scene == domain.SceneLogin {
		failCount := u.getFailCountByIP(ctx, cmd.IP)
		identifierType := "ip"
		identifierValue := cmd.IP

		if cmd.UserID != "" {
			failCount = u.getFailCountByUserID(ctx, cmd.UserID)
			identifierType = "userId"
			identifierValue = cmd.UserID
		}

		if failCount >= int64(u.Options.Login.MaxFailCount) {
			u.Logger.Info("login brute-force protection triggered",
				zap.String("identifierType", identifierType),
				zap.String("identifierValue", identifierValue),
				zap.Int64("count", failCount))
			return finish(ports.CheckDecision{Action: domain.ActionVerify, Reason: "TOO_MANY_FAILED_ATTEMPTS"})
		}
	}

	return finish(ports.CheckDecision{Action: domain.ActionPass, Reason: "PASS"})
}

func (u *RiskUseCase) ReportEvent(ctx context.Context, cmd ports.ReportEventCommand) (ports.ReportEventResult, error) {
	if cmd.Scene != domain.SceneLogin {
		return ports.ReportEventResult{}, domain.ErrUnsupportedScene
	}

	target, identifierType, identifierValue, err := resolveLoginFailureTarget(cmd.UserID, cmd.IP)
	if err != nil {
		return ports.ReportEventResult{}, domain.ErrLoginIdentityEmpty
	}

	finish := func(received bool, count int64) (ports.ReportEventResult, error) {
		if cmd.IP != "" && u.Insights != nil {
			if err := u.Insights.RecordReportEvent(ctx, domain.RiskReportInsight{
				ReqID:          cmd.ReqID,
				Scene:          cmd.Scene,
				IP:             cmd.IP,
				UserID:         cmd.UserID,
				IsSuccess:      cmd.IsSuccess,
				Received:       received,
				LoginFailCount: count,
			}); err != nil {
				u.Logger.Warn("failed to record risk ip report insight", zap.Error(err), zap.String("ip", cmd.IP))
			}
		}
		return ports.ReportEventResult{Received: received}, nil
	}

	if cmd.IsSuccess {
		if err := u.LoginFailure.Clear(ctx, target); err != nil {
			u.Logger.Error("failed to clear login fail count",
				zap.Error(err),
				zap.String("identifierType", identifierType),
				zap.String("identifierValue", identifierValue))
			return finish(false, 0)
		}

		u.Logger.Info("login success cleared fail count",
			zap.String("identifierType", identifierType),
			zap.String("identifierValue", identifierValue))
		return finish(true, 0)
	}

	count, err := u.LoginFailure.RecordFailure(ctx, target, u.loginFailureExpireSeconds())
	if err != nil {
		u.Logger.Error("failed to record login failure",
			zap.Error(err),
			zap.String("identifierType", identifierType),
			zap.String("identifierValue", identifierValue))
		return finish(false, 0)
	}

	u.Logger.Info("login failure recorded",
		zap.String("identifierType", identifierType),
		zap.String("identifierValue", identifierValue),
		zap.Int64("count", count))
	return finish(true, count)
}

func (u *RiskUseCase) AddBlacklist(ctx context.Context, cmd ports.AddBlacklistCommand) (ports.AddBlacklistResult, error) {
	if err := u.Blacklist.Save(ctx, domain.BlacklistEntry{
		Type:     cmd.Type,
		Value:    cmd.Value,
		Reason:   cmd.Reason,
		ExpireAt: cmd.ExpireAt,
	}); err != nil {
		return ports.AddBlacklistResult{}, err
	}

	if u.Insights != nil {
		if err := u.Insights.RecordBlacklist(ctx, domain.RiskBlacklistInsight{
			Type:     cmd.Type,
			Value:    cmd.Value,
			Reason:   cmd.Reason,
			ExpireAt: cmd.ExpireAt,
		}); err != nil {
			u.Logger.Warn("failed to record risk ip blacklist insight", zap.Error(err), zap.String("value", cmd.Value))
		}
	}

	u.Logger.Info("blacklist item added",
		zap.String("key", fmt.Sprintf("risk:blacklist:%s:%s", cmd.Type, cmd.Value)),
		zap.String("reason", cmd.Reason))
	return ports.AddBlacklistResult{Success: true}, nil
}

func (u *RiskUseCase) OnlineSelfTest(ctx context.Context, cmd ports.UserActionCommand) (ports.UserActionResult, error) {
	if cmd.UserID == "" {
		return ports.UserActionResult{}, domain.ErrUserIDEmpty
	}

	if u.checkUserRateLimit(ctx, cmd.UserID, ScopeOnlineSelfTest, u.Options.UserRateLimit.OnlineSelfTest) {
		return ports.UserActionResult{}, domain.ErrRateLimitExceeded
	}

	return ports.UserActionResult{Accepted: true, Reason: "PASS"}, nil
}

func (u *RiskUseCase) JudgeSubmission(ctx context.Context, cmd ports.UserActionCommand) (ports.UserActionResult, error) {
	if cmd.UserID == "" {
		return ports.UserActionResult{}, domain.ErrUserIDEmpty
	}

	if u.checkUserRateLimit(ctx, cmd.UserID, ScopeJudgeSubmission, u.Options.UserRateLimit.JudgeSubmission) {
		return ports.UserActionResult{}, domain.ErrRateLimitExceeded
	}

	return ports.UserActionResult{Accepted: true, Reason: "PASS"}, nil
}

func (u *RiskUseCase) ListRiskIPs(ctx context.Context, query ports.RiskIPListQuery) (*ports.RiskIPListResponse, error) {
	return u.Insights.ListRiskIPs(ctx, query, int64(u.Options.Login.MaxFailCount))
}

func (u *RiskUseCase) GetRiskIP(ctx context.Context, ip string) (*domain.RiskIPDetail, error) {
	return u.Insights.GetRiskIP(ctx, ip, int64(u.Options.Login.MaxFailCount))
}

func (u *RiskUseCase) ListRiskIPEvents(ctx context.Context, ip string, query ports.RiskIPEventsQuery) (*ports.RiskIPEventsResponse, error) {
	return u.Insights.ListRiskIPEvents(ctx, ip, query)
}

func (u *RiskUseCase) checkBlacklist(ctx context.Context, cmd ports.CheckCommand) (bool, string) {
	if u.Blacklist == nil {
		return false, ""
	}

	if entry, found, err := u.Blacklist.Get(ctx, domain.BlacklistTypeIP, cmd.IP); err == nil && found {
		return true, "BLACKLIST_IP: " + entry.Reason
	}

	if cmd.UserID != "" {
		if entry, found, err := u.Blacklist.Get(ctx, domain.BlacklistTypeUserID, cmd.UserID); err == nil && found {
			return true, "BLACKLIST_UID: " + entry.Reason
		}
	}

	return false, ""
}

func (u *RiskUseCase) checkIPRateLimit(ctx context.Context, ip string) bool {
	if u.RateLimiter == nil {
		return false
	}

	result, err := u.RateLimiter.IncrementIP(ctx, ip, u.Options.IPRateLimit)
	if err != nil {
		u.Logger.Error("ip rate limit check failed", zap.Error(err))
		return false
	}

	if result.Exceeded {
		u.Logger.Warn("ip rate limit triggered", zap.String("ip", ip), zap.Int64("count", result.Count))
		return true
	}
	return false
}

func (u *RiskUseCase) checkUserRateLimit(ctx context.Context, userID string, scope string, rule domain.RateLimitRule) bool {
	if userID == "" || u.RateLimiter == nil {
		return false
	}

	result, err := u.RateLimiter.IncrementUser(ctx, userID, scope, rule)
	if err != nil {
		u.Logger.Error("user rate limit check failed", zap.Error(err), zap.String("scope", scope))
		return false
	}

	if result.Exceeded {
		u.Logger.Warn("user rate limit triggered",
			zap.String("userId", userID),
			zap.String("scope", scope),
			zap.Int64("count", result.Count))
		return true
	}
	return false
}

func (u *RiskUseCase) getFailCountByUserID(ctx context.Context, userID string) int64 {
	if u.LoginFailure == nil {
		return 0
	}
	val, err := u.LoginFailure.GetByUserID(ctx, userID)
	if err != nil {
		return 0
	}
	return val
}

func (u *RiskUseCase) getFailCountByIP(ctx context.Context, ip string) int64 {
	if u.LoginFailure == nil {
		return 0
	}
	val, err := u.LoginFailure.GetByIP(ctx, ip)
	if err != nil {
		return 0
	}
	return val
}

func (u *RiskUseCase) loginFailureExpireSeconds() int {
	return u.Options.Login.FailCountExpireMinutes * int(time.Minute/time.Second)
}

func resolveLoginFailureTarget(userID string, ip string) (domain.LoginFailureTarget, string, string, error) {
	if userID != "" {
		return domain.LoginFailureTarget{Type: domain.LoginFailureTargetUserID, Value: userID}, "userId", userID, nil
	}

	if ip != "" {
		return domain.LoginFailureTarget{Type: domain.LoginFailureTargetIP, Value: ip}, "ip", ip, nil
	}

	return domain.LoginFailureTarget{}, "", "", fmt.Errorf("missing userId and ip")
}
