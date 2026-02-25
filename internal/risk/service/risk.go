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

// RiskService 结构体现在包含了配置和日志
type RiskService struct {
	pb.UnimplementedRiskControlServiceServer
	Rdb    *redis.Client
	Config *config.RiskRulesConfig
	Logger *zap.Logger
}

// NewRiskService - RiskService 的构造函数
func NewRiskService(rdb *redis.Client, cfg *config.RiskRulesConfig, logger *zap.Logger) *RiskService {
	return &RiskService{
		Rdb:    rdb,
		Config: cfg,
		Logger: logger,
	}
}

// Check 核心检测逻辑
func (s *RiskService) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	// 生产环境增加基础校验
	if req.Ip == "" {
		return &pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: "INVALID_REQUEST_IP_EMPTY"}, nil
	}

	// 1. 黑名单检测
	if hit, reason := s.checkBlacklist(ctx, req); hit {
		s.Logger.Warn("命中黑名单", zap.String("ip", req.Ip), zap.String("userId", req.UserId), zap.String("reason", reason))
		return &pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: reason}, nil
	}

	// 2. IP 频控 (使用配置)
	if s.checkIpRateLimit(ctx, req.Ip) {
		return &pb.CheckResponse{Action: pb.Action_ACTION_REJECT, Reason: "IP_RATE_LIMIT_EXCEEDED"}, nil
	}

	// 3. 防暴力破解检测 (使用配置) - 优先检查用户ID
	if req.Scene == pb.Scene_SCENE_LOGIN {
		// 优先使用用户ID进行防暴力破解检测
		if req.UserId != "" {
			if s.getFailCountByUserId(ctx, req.UserId) > int64(s.Config.Login.MaxFailCount) {
				s.Logger.Info("触发防爆破规则", zap.String("userId", req.UserId))
				return &pb.CheckResponse{Action: pb.Action_ACTION_VERIFY, Reason: "TOO_MANY_FAILED_ATTEMPTS"}, nil
			}
		}
		// TODO: 未来可扩展基于IP的防暴力破解检测
		// if s.getFailCountByIp(ctx, req.Ip) > int64(s.Config.Login.MaxFailCount) {
		// 	s.Logger.Info("触发防爆破规则(IP)", zap.String("ip", req.Ip))
		// 	return &pb.CheckResponse{Action: pb.Action_ACTION_VERIFY, Reason: "TOO_MANY_FAILED_ATTEMPTS_IP"}, nil
		// }
	}

	return &pb.CheckResponse{Action: pb.Action_ACTION_PASS, Reason: "PASS"}, nil
}

// ReportEvent 核心上报逻辑 - 优先处理用户ID
func (s *RiskService) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
	if req.Scene == pb.Scene_SCENE_LOGIN && req.UserId != "" {
		key := fmt.Sprintf("risk:fail_count:login:uid:%s", req.UserId)

		if req.IsSuccess {
			// 登录成功，删除失败计数
			err := s.Rdb.Del(ctx, key).Err()
			if err != nil {
				s.Logger.Error("清除失败计数失败", zap.Error(err), zap.String("userId", req.UserId))
			} else {
				s.Logger.Info("登录成功，已清除失败计数", zap.String("userId", req.UserId))
			}
		} else {
			// 登录失败，增加计数
			// 使用 Pipeline 提高性能，减少网络往返
			pipe := s.Rdb.Pipeline()
			pipe.Incr(ctx, key)
			pipe.Expire(ctx, key, time.Duration(s.Config.Login.FailCountExpireMinutes)*time.Minute)
			_, err := pipe.Exec(ctx)
			if err != nil {
				s.Logger.Error("上报事件 Redis 操作失败(用户ID)", zap.Error(err), zap.String("userId", req.UserId))
				// 上报失败不应阻塞主流程，直接返回成功
			} else {
				s.Logger.Info("记录登录失败事件", zap.String("userId", req.UserId))
			}
		}

		// TODO: 未来可扩展基于IP的上报逻辑
		// if req.Ip != "" {
		// 	ipKey := fmt.Sprintf("risk:fail_count:login:ip:%s", req.Ip)
		// 	if req.IsSuccess {
		// 		s.Rdb.Del(ctx, ipKey)
		// 	} else {
		// 		pipe := s.Rdb.Pipeline()
		// 		pipe.Incr(ctx, ipKey)
		// 		pipe.Expire(ctx, ipKey, time.Duration(s.Config.Login.FailCountExpireMinutes)*time.Minute)
		// 		pipe.Exec(ctx)
		// 	}
		// }
	}
	return &pb.ReportEventResponse{Received: true}, nil
}

// AddBlacklist 添加黑名单
func (s *RiskService) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) (*pb.AddBlacklistResponse, error) {
	// 构造 Key：risk:blacklist:ip:127.0.0.1
	keyType := "unknown"
	switch req.Type {
	case pb.AddBlacklistRequest_TYPE_IP:
		keyType = "ip"
	case pb.AddBlacklistRequest_TYPE_USER_ID:
		keyType = "uid"
	}

	key := fmt.Sprintf("risk:blacklist:%s:%s", keyType, req.Value)

	// 设置到 Redis
	var expiration time.Duration
	if req.ExpireAt > 0 {
		// 计算剩余时间
		expiration = time.Until(time.Unix(req.ExpireAt, 0))
		if expiration < 0 {
			expiration = time.Second // 已经过期了
		}
	} else {
		expiration = 0 // 永久
	}

	err := s.Rdb.Set(ctx, key, req.Reason, expiration).Err()
	if err != nil {
		return nil, err
	}

	s.Logger.Info("已添加黑名单", zap.String("key", key), zap.String("reason", req.Reason))
	return &pb.AddBlacklistResponse{Success: true}, nil
}

// OnlineSelfTest 在线自测
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

// JudgeSubmission 提交题目判题
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

// 内部辅助方法

// checkBlacklist 检查黑名单
func (s *RiskService) checkBlacklist(ctx context.Context, req *pb.CheckRequest) (bool, string) {
	// 1. 查 IP 黑名单
	ipKey := fmt.Sprintf("risk:blacklist:ip:%s", req.Ip)
	if val, err := s.Rdb.Get(ctx, ipKey).Result(); err == nil {
		return true, "BLACKLIST_IP: " + val
	}

	// 2. 查 UserID 黑名单 (如果有 UserID)
	if req.UserId != "" {
		uidKey := fmt.Sprintf("risk:blacklist:uid:%s", req.UserId)
		if val, err := s.Rdb.Get(ctx, uidKey).Result(); err == nil {
			return true, "BLACKLIST_UID: " + val
		}
	}

	return false, ""
}

// 1. 定义 Lua 脚本
var ipRateLimitLua = redis.NewScript(`
    local current = redis.call("INCR", KEYS[1])
    if current == 1 then
        redis.call("EXPIRE", KEYS[1], ARGV[1])
    end
    return current
`)

func (s *RiskService) checkIpRateLimit(ctx context.Context, ip string) bool {
	key := fmt.Sprintf("risk:rate:ip:%s", ip)
	limit := int64(s.Config.IpRateLimit.Limit)
	windowSeconds := s.Config.IpRateLimit.WindowSeconds

	// 2. 执行 Lua 脚本 (将 INCR 和 EXPIRE 合并为一个原子操作)
	// KEYS[1] 是 key, ARGV[1] 是过期时间
	val, err := ipRateLimitLua.Run(ctx, s.Rdb, []string{key}, windowSeconds).Int64()
	if err != nil {
		s.Logger.Error("Lua频控检查失败", zap.Error(err))
		return false
	}

	if val > limit {
		s.Logger.Warn("IP 触发频控", zap.String("ip", ip), zap.Int64("count", val))
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
		s.Logger.Error("Lua频控检查失败", zap.Error(err), zap.String("scope", scope))
		return false
	}

	if val > limit {
		s.Logger.Warn("用户触发频控", zap.String("userId", userID), zap.String("scope", scope), zap.Int64("count", val))
		return true
	}
	return false
}

// getFailCountByUserId 获取用户的失败计数
func (s *RiskService) getFailCountByUserId(ctx context.Context, userId string) int64 {
	key := fmt.Sprintf("risk:fail_count:login:uid:%s", userId)
	val, err := s.Rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}

// getFailCountByIp 获取IP的失败计数 (保留供未来扩展)
func (s *RiskService) getFailCountByIp(ctx context.Context, ip string) int64 {
	key := fmt.Sprintf("risk:fail_count:login:ip:%s", ip)
	val, err := s.Rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}
