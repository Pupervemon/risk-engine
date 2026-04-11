package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RiskService struct {
	pb.UnimplementedRiskControlServiceServer
	Rdb    *redis.Client
	Config *config.RiskRulesConfig
	Logger *zap.Logger
}

func NewRiskService(rdb *redis.Client, cfg *config.RiskRulesConfig, logger *zap.Logger) *RiskService {
	return &RiskService{
		Rdb:    rdb,
		Config: cfg,
		Logger: logger,
	}
}

func (s *RiskService) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	finish := func(resp *pb.CheckResponse) (*pb.CheckResponse, error) {
		if req.Ip != "" {
			if err := s.recordCheckInsight(ctx, req, resp); err != nil {
				s.Logger.Warn("failed to record risk ip check insight", zap.Error(err), zap.String("ip", req.Ip))
			}
		}
		return resp, nil
	}

	if req.Ip == "" {
		return finish(&pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: "INVALID_REQUEST_IP_EMPTY"})
	}

	if hit, reason := s.checkBlacklist(ctx, req); hit {
		s.Logger.Warn("blacklist hit",
			zap.String("ip", req.Ip),
			zap.String("userId", req.UserId),
			zap.String("reason", reason))
		return finish(&pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: reason})
	}

	if s.checkIpRateLimit(ctx, req.Ip) {
		return finish(&pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: "IP_RATE_LIMIT_EXCEEDED"})
	}

	if req.Scene == pb.Scene_SCENE_LOGIN {
		failCount := s.getFailCountByIp(ctx, req.Ip)
		identifierType := "ip"
		identifierValue := req.Ip

		if req.UserId != "" {
			failCount = s.getFailCountByUserId(ctx, req.UserId)
			identifierType = "userId"
			identifierValue = req.UserId
		}

		if failCount >= int64(s.Config.Login.MaxFailCount) {
			s.Logger.Info("login brute-force protection triggered",
				zap.String("identifierType", identifierType),
				zap.String("identifierValue", identifierValue),
				zap.Int64("count", failCount))
			return finish(&pb.CheckResponse{Action: pb.Action_ACTION_VERIFY, Reason: "TOO_MANY_FAILED_ATTEMPTS"})
		}
	}

	return finish(&pb.CheckResponse{Action: pb.Action_ACTION_PASS, Reason: "PASS"})
}

func (s *RiskService) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	if req.Scene != pb.Scene_SCENE_LOGIN {
		return nil, status.Error(codes.InvalidArgument, "UNSUPPORTED_SCENE")
	}

	key, identifierType, identifierValue, err := resolveLoginFailCountTarget(req.UserId, req.Ip)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "LOGIN_EVENT_IDENTITY_EMPTY")
	}

	finish := func(received bool, count int64) (*pb.ReportEventResponse, error) {
		if req.Ip != "" {
			if err := s.recordReportEventInsight(ctx, req, received, count); err != nil {
				s.Logger.Warn("failed to record risk ip report insight", zap.Error(err), zap.String("ip", req.Ip))
			}
		}
		return &pb.ReportEventResponse{Received: received}, nil
	}

	if req.IsSuccess {
		if err := s.Rdb.Del(ctx, key).Err(); err != nil {
			s.Logger.Error("failed to clear login fail count",
				zap.Error(err),
				zap.String("identifierType", identifierType),
				zap.String("identifierValue", identifierValue))
			return finish(false, 0)
		}

		s.Logger.Info("login success cleared fail count",
			zap.String("identifierType", identifierType),
			zap.String("identifierValue", identifierValue))
		return finish(true, 0)
	}

	count, err := s.recordLoginFailure(ctx, key)
	if err != nil {
		s.Logger.Error("failed to record login failure",
			zap.Error(err),
			zap.String("identifierType", identifierType),
			zap.String("identifierValue", identifierValue))
		return finish(false, 0)
	}

	s.Logger.Info("login failure recorded",
		zap.String("identifierType", identifierType),
		zap.String("identifierValue", identifierValue),
		zap.Int64("count", count))
	return finish(true, count)
}

func (s *RiskService) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) (*pb.AddBlacklistResponse, error) {
	keyType := "unknown"
	switch req.Type {
	case pb.AddBlacklistRequest_TYPE_IP:
		keyType = "ip"
	case pb.AddBlacklistRequest_TYPE_USER_ID:
		keyType = "uid"
	}

	key := fmt.Sprintf("risk:blacklist:%s:%s", keyType, req.Value)

	var expiration time.Duration
	if req.ExpireAt > 0 {
		expiration = time.Until(time.Unix(req.ExpireAt, 0))
		if expiration < 0 {
			expiration = time.Second
		}
	}

	if err := s.Rdb.Set(ctx, key, req.Reason, expiration).Err(); err != nil {
		return nil, err
	}

	if err := s.recordBlacklistInsight(ctx, req); err != nil {
		s.Logger.Warn("failed to record risk ip blacklist insight", zap.Error(err), zap.String("value", req.Value))
	}

	s.Logger.Info("blacklist item added", zap.String("key", key), zap.String("reason", req.Reason))
	return &pb.AddBlacklistResponse{Success: true}, nil
}

func (s *RiskService) OnlineSelfTest(ctx context.Context, req *pb.OnlineSelfTestRequest) (*pb.OnlineSelfTestResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "USER_ID_EMPTY")
	}

	rule := s.Config.UserRateLimit.OnlineSelfTest
	if s.checkUserRateLimit(ctx, req.UserId, "online_self_test", rule) {
		return nil, status.Error(codes.ResourceExhausted, "USER_RATE_LIMIT_EXCEEDED")
	}

	return &pb.OnlineSelfTestResponse{Accepted: true, Reason: "PASS"}, nil
}

func (s *RiskService) JudgeSubmission(ctx context.Context, req *pb.JudgeSubmissionRequest) (*pb.JudgeSubmissionResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "USER_ID_EMPTY")
	}

	rule := s.Config.UserRateLimit.JudgeSubmission
	if s.checkUserRateLimit(ctx, req.UserId, "judge_submission", rule) {
		return nil, status.Error(codes.ResourceExhausted, "USER_RATE_LIMIT_EXCEEDED")
	}

	return &pb.JudgeSubmissionResponse{Accepted: true, Reason: "PASS"}, nil
}

func (s *RiskService) checkBlacklist(ctx context.Context, req *pb.CheckRequest) (bool, string) {
	ipKey := fmt.Sprintf("risk:blacklist:ip:%s", req.Ip)
	if val, err := s.Rdb.Get(ctx, ipKey).Result(); err == nil {
		return true, "BLACKLIST_IP: " + val
	}

	if req.UserId != "" {
		uidKey := fmt.Sprintf("risk:blacklist:uid:%s", req.UserId)
		if val, err := s.Rdb.Get(ctx, uidKey).Result(); err == nil {
			return true, "BLACKLIST_UID: " + val
		}
	}

	return false, ""
}

var ipRateLimitLua = redis.NewScript(`
    local current = redis.call("INCR", KEYS[1])
    if current == 1 then
        redis.call("EXPIRE", KEYS[1], ARGV[1])
    end
    return current
`)

var loginFailCountLua = redis.NewScript(`
    local current = redis.call("INCR", KEYS[1])
    redis.call("EXPIRE", KEYS[1], ARGV[1])
    return current
`)

func (s *RiskService) checkIpRateLimit(ctx context.Context, ip string) bool {
	key := fmt.Sprintf("risk:rate:ip:%s", ip)
	limit := int64(s.Config.IpRateLimit.Limit)
	windowSeconds := s.Config.IpRateLimit.WindowSeconds

	val, err := ipRateLimitLua.Run(ctx, s.Rdb, []string{key}, windowSeconds).Int64()
	if err != nil {
		s.Logger.Error("ip rate limit check failed", zap.Error(err))
		return false
	}

	if val > limit {
		s.Logger.Warn("ip rate limit triggered", zap.String("ip", ip), zap.Int64("count", val))
		return true
	}
	return false
}

func (s *RiskService) checkUserRateLimit(ctx context.Context, userID string, scope string, rule config.UserRateLimitRule) bool {
	if userID == "" {
		return false
	}

	key := fmt.Sprintf("risk:rate:user:%s:%s", userID, scope)
	limit := int64(rule.Limit)
	windowSeconds := rule.WindowSeconds

	val, err := ipRateLimitLua.Run(ctx, s.Rdb, []string{key}, windowSeconds).Int64()
	if err != nil {
		s.Logger.Error("user rate limit check failed", zap.Error(err), zap.String("scope", scope))
		return false
	}

	if val > limit {
		s.Logger.Warn("user rate limit triggered",
			zap.String("userId", userID),
			zap.String("scope", scope),
			zap.Int64("count", val))
		return true
	}
	return false
}

func (s *RiskService) getFailCountByUserId(ctx context.Context, userId string) int64 {
	key := loginFailCountKeyByUserID(userId)
	val, err := s.Rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

func (s *RiskService) getFailCountByIp(ctx context.Context, ip string) int64 {
	key := loginFailCountKeyByIP(ip)
	val, err := s.Rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

func (s *RiskService) recordLoginFailure(ctx context.Context, key string) (int64, error) {
	expireSeconds := s.Config.Login.FailCountExpireMinutes * int(time.Minute/time.Second)
	if expireSeconds <= 0 {
		return 0, fmt.Errorf("invalid fail_count_expire_minutes: %d", s.Config.Login.FailCountExpireMinutes)
	}

	return loginFailCountLua.Run(ctx, s.Rdb, []string{key}, expireSeconds).Int64()
}

func resolveLoginFailCountTarget(userID string, ip string) (string, string, string, error) {
	if userID != "" {
		return loginFailCountKeyByUserID(userID), "userId", userID, nil
	}

	if ip != "" {
		return loginFailCountKeyByIP(ip), "ip", ip, nil
	}

	return "", "", "", fmt.Errorf("missing userId and ip")
}

func loginFailCountKeyByUserID(userID string) string {
	return fmt.Sprintf("risk:fail_count:login:uid:%s", userID)
}

func loginFailCountKeyByIP(ip string) string {
	return fmt.Sprintf("risk:fail_count:login:ip:%s", ip)
}
