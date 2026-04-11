package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	riskIPInsightIndexKey      = "risk:insight:ip:index:last_seen"
	riskIPInsightProfilePrefix = "risk:insight:ip:profile:"
	riskIPInsightEventsPrefix  = "risk:insight:ip:events:"
	riskIPInsightLoginFailKey  = "risk:insight:ip:login_fail:"
	riskIPInsightTTL           = 30 * 24 * time.Hour
	riskIPInsightEventLimit    = 200
	riskIPListDefaultLimit     = 20
	riskIPListMaxLimit         = 100
	riskIPEventsDefaultLimit   = 50
	riskIPEventsMaxLimit       = 200
)

var (
	ErrRiskIPNotFound = errors.New("risk ip not found")
	ErrInvalidRiskIP  = errors.New("invalid risk ip")
)

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
	Items   []RiskIPSummary `json:"items"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
}

type RiskIPEventsResponse struct {
	IP      string        `json:"ip"`
	Items   []RiskIPEvent `json:"items"`
	Offset  int           `json:"offset"`
	Limit   int           `json:"limit"`
	Total   int64         `json:"total"`
	HasMore bool          `json:"has_more"`
}

type RiskIPSummary struct {
	IP                       string   `json:"ip"`
	FirstSeenAt              string   `json:"first_seen_at,omitempty"`
	LastSeenAt               string   `json:"last_seen_at,omitempty"`
	LastScene                string   `json:"last_scene,omitempty"`
	LastAction               string   `json:"last_action,omitempty"`
	LastReason               string   `json:"last_reason,omitempty"`
	LastReqID                string   `json:"last_req_id,omitempty"`
	LatestUserIDMasked       string   `json:"latest_user_id_masked,omitempty"`
	TotalChecks              int64    `json:"total_checks"`
	TotalRejects             int64    `json:"total_rejects"`
	TotalVerifies            int64    `json:"total_verifies"`
	TotalReportSuccess       int64    `json:"total_report_success"`
	TotalReportFailure       int64    `json:"total_report_failure"`
	TotalBlacklistHits       int64    `json:"total_blacklist_hits"`
	CurrentLoginFailCount    int64    `json:"current_login_fail_count"`
	CurrentLoginFailExpireAt string   `json:"current_login_fail_expire_at,omitempty"`
	Blacklisted              bool     `json:"blacklisted"`
	BlacklistReason          string   `json:"blacklist_reason,omitempty"`
	BlacklistExpireAt        string   `json:"blacklist_expire_at,omitempty"`
	Flags                    []string `json:"flags"`
	Severity                 string   `json:"severity"`
}

type RiskIPDetail struct {
	RiskIPSummary
	EventCount int64 `json:"event_count"`
}

type RiskIPEvent struct {
	EventID            string `json:"event_id"`
	EventType          string `json:"event_type"`
	IP                 string `json:"ip"`
	Scene              string `json:"scene,omitempty"`
	Action             string `json:"action,omitempty"`
	Reason             string `json:"reason,omitempty"`
	ReqID              string `json:"req_id,omitempty"`
	UserIDMasked       string `json:"user_id_masked,omitempty"`
	Received           *bool  `json:"received,omitempty"`
	LoginFailCount     int64  `json:"login_fail_count,omitempty"`
	OccurredAt         string `json:"occurred_at"`
	OccurredAtUnix     int64  `json:"occurred_at_unix"`
	BlacklistExpireAt  string `json:"blacklist_expire_at,omitempty"`
	CurrentBlacklisted bool   `json:"current_blacklisted,omitempty"`
}

func (s *RiskService) ListRiskIPs(ctx context.Context, query RiskIPListQuery) (*RiskIPListResponse, error) {
	limit := clampInt(query.Limit, riskIPListDefaultLimit, riskIPListMaxLimit)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	search := strings.TrimSpace(query.Search)
	if search != "" {
		detail, err := s.GetRiskIP(ctx, search)
		if err != nil {
			if errors.Is(err, ErrRiskIPNotFound) {
				return &RiskIPListResponse{
					Items:   []RiskIPSummary{},
					Offset:  0,
					Limit:   limit,
					Total:   0,
					HasMore: false,
				}, nil
			}
			return nil, err
		}

		return &RiskIPListResponse{
			Items:   []RiskIPSummary{detail.RiskIPSummary},
			Offset:  0,
			Limit:   limit,
			Total:   1,
			HasMore: false,
		}, nil
	}

	total, err := s.Rdb.ZCard(ctx, riskIPInsightIndexKey).Result()
	if err != nil {
		return nil, err
	}

	members, err := s.Rdb.ZRevRange(ctx, riskIPInsightIndexKey, int64(offset), int64(offset+limit)).Result()
	if err != nil {
		return nil, err
	}

	hasMore := len(members) > limit
	if hasMore {
		members = members[:limit]
	}

	items := make([]RiskIPSummary, 0, len(members))
	for _, ip := range members {
		detail, err := s.GetRiskIP(ctx, ip)
		if err != nil {
			if errors.Is(err, ErrRiskIPNotFound) {
				_ = s.Rdb.ZRem(ctx, riskIPInsightIndexKey, ip).Err()
				continue
			}
			return nil, err
		}
		items = append(items, detail.RiskIPSummary)
	}

	return &RiskIPListResponse{
		Items:   items,
		Offset:  offset,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *RiskService) GetRiskIP(ctx context.Context, ip string) (*RiskIPDetail, error) {
	normalizedIP, err := normalizeRiskIP(ip)
	if err != nil {
		return nil, err
	}

	summary, found, err := s.loadRiskIPSummary(ctx, normalizedIP)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrRiskIPNotFound
	}

	eventCount, err := s.Rdb.LLen(ctx, riskIPInsightEventsKey(normalizedIP)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	return &RiskIPDetail{
		RiskIPSummary: *summary,
		EventCount:    eventCount,
	}, nil
}

func (s *RiskService) ListRiskIPEvents(ctx context.Context, ip string, query RiskIPEventsQuery) (*RiskIPEventsResponse, error) {
	normalizedIP, err := normalizeRiskIP(ip)
	if err != nil {
		return nil, err
	}

	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	limit := clampInt(query.Limit, riskIPEventsDefaultLimit, riskIPEventsMaxLimit)

	eventsKey := riskIPInsightEventsKey(normalizedIP)
	total, err := s.Rdb.LLen(ctx, eventsKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	if total == 0 {
		if _, err := s.GetRiskIP(ctx, normalizedIP); err != nil {
			return nil, err
		}
		return &RiskIPEventsResponse{
			IP:      normalizedIP,
			Items:   []RiskIPEvent{},
			Offset:  offset,
			Limit:   limit,
			Total:   0,
			HasMore: false,
		}, nil
	}

	values, err := s.Rdb.LRange(ctx, eventsKey, int64(offset), int64(offset+limit)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	hasMore := len(values) > limit
	if hasMore {
		values = values[:limit]
	}

	items := make([]RiskIPEvent, 0, len(values))
	for _, raw := range values {
		var event RiskIPEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			s.Logger.Warn("failed to decode risk ip event", zap.Error(err), zap.String("ip", normalizedIP))
			continue
		}
		items = append(items, event)
	}

	return &RiskIPEventsResponse{
		IP:      normalizedIP,
		Items:   items,
		Offset:  offset,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *RiskService) recordCheckInsight(ctx context.Context, req *pb.CheckRequest, resp *pb.CheckResponse) error {
	if req == nil || resp == nil || req.Ip == "" {
		return nil
	}

	fields := map[string]interface{}{
		"last_scene":  sceneLabel(req.Scene),
		"last_action": actionLabel(resp.Action),
		"last_reason": resp.Reason,
	}
	if req.ReqId != "" {
		fields["last_req_id"] = req.ReqId
	}
	if masked := maskUserID(req.UserId); masked != "" {
		fields["latest_user_id_masked"] = masked
	}

	increments := map[string]int64{
		"total_checks": 1,
	}
	switch resp.Action {
	case pb.Action_ACTION_REJECT:
		increments["total_rejects"] = 1
	case pb.Action_ACTION_VERIFY:
		increments["total_verifies"] = 1
	}
	if strings.HasPrefix(resp.Reason, "BLACKLIST_") {
		increments["total_blacklist_hits"] = 1
	}

	event := RiskIPEvent{
		EventID:        newRiskIPEventID("check"),
		EventType:      "check",
		IP:             req.Ip,
		Scene:          sceneLabel(req.Scene),
		Action:         actionLabel(resp.Action),
		Reason:         resp.Reason,
		ReqID:          req.ReqId,
		UserIDMasked:   maskUserID(req.UserId),
		OccurredAt:     time.Now().UTC().Format(time.RFC3339),
		OccurredAtUnix: time.Now().UTC().Unix(),
	}

	return s.writeRiskIPInsight(ctx, req.Ip, fields, increments, &event)
}

func (s *RiskService) recordReportEventInsight(ctx context.Context, req *pb.ReportEventRequest, received bool, loginFailCount int64) error {
	if req == nil || req.Ip == "" {
		return nil
	}

	insightFailCount := loginFailCount
	if req.IsSuccess {
		if err := s.Rdb.Del(ctx, riskIPInsightLoginFailCounterKey(req.Ip)).Err(); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		insightFailCount = 0
	} else {
		count, err := s.recordRiskIPLoginFailCounter(ctx, req.Ip)
		if err != nil {
			return err
		}
		insightFailCount = count
	}

	action := "failure"
	reason := "LOGIN_FAILURE"
	increments := map[string]int64{
		"total_report_failure": 1,
	}
	if req.IsSuccess {
		action = "success"
		reason = "LOGIN_SUCCESS"
		increments = map[string]int64{
			"total_report_success": 1,
		}
	}

	fields := map[string]interface{}{
		"last_scene":  sceneLabel(req.Scene),
		"last_action": "report_" + action,
		"last_reason": reason,
	}
	if req.ReqId != "" {
		fields["last_req_id"] = req.ReqId
	}
	if masked := maskUserID(req.UserId); masked != "" {
		fields["latest_user_id_masked"] = masked
	}

	event := RiskIPEvent{
		EventID:        newRiskIPEventID("report"),
		EventType:      "report_" + action,
		IP:             req.Ip,
		Scene:          sceneLabel(req.Scene),
		Action:         action,
		Reason:         reason,
		ReqID:          req.ReqId,
		UserIDMasked:   maskUserID(req.UserId),
		Received:       boolPtr(received),
		LoginFailCount: insightFailCount,
		OccurredAt:     time.Now().UTC().Format(time.RFC3339),
		OccurredAtUnix: time.Now().UTC().Unix(),
	}

	return s.writeRiskIPInsight(ctx, req.Ip, fields, increments, &event)
}

func (s *RiskService) recordBlacklistInsight(ctx context.Context, req *pb.AddBlacklistRequest) error {
	if req == nil || req.Type != pb.AddBlacklistRequest_TYPE_IP || req.Value == "" {
		return nil
	}

	expireAt := ""
	if req.ExpireAt > 0 {
		expireAt = time.Unix(req.ExpireAt, 0).UTC().Format(time.RFC3339)
	}

	fields := map[string]interface{}{
		"last_action": "blacklist_added",
		"last_reason": req.Reason,
	}

	event := RiskIPEvent{
		EventID:            newRiskIPEventID("blacklist"),
		EventType:          "blacklist_added",
		IP:                 req.Value,
		Action:             "blacklist_added",
		Reason:             req.Reason,
		OccurredAt:         time.Now().UTC().Format(time.RFC3339),
		OccurredAtUnix:     time.Now().UTC().Unix(),
		BlacklistExpireAt:  expireAt,
		CurrentBlacklisted: true,
	}

	return s.writeRiskIPInsight(ctx, req.Value, fields, nil, &event)
}

func (s *RiskService) writeRiskIPInsight(ctx context.Context, ip string, fields map[string]interface{}, increments map[string]int64, event *RiskIPEvent) error {
	normalizedIP, err := normalizeRiskIP(ip)
	if err != nil {
		return nil
	}

	now := time.Now().UTC()
	profileKey := riskIPInsightProfileKey(normalizedIP)
	eventsKey := riskIPInsightEventsKey(normalizedIP)

	pipe := s.Rdb.TxPipeline()
	pipe.HSetNX(ctx, profileKey, "first_seen_at", now.Unix())
	pipe.HSet(ctx, profileKey, "last_seen_at", now.Unix())

	if len(fields) > 0 {
		pipe.HSet(ctx, profileKey, fields)
	}
	for field, delta := range increments {
		if delta != 0 {
			pipe.HIncrBy(ctx, profileKey, field, delta)
		}
	}
	pipe.Expire(ctx, profileKey, riskIPInsightTTL)
	pipe.ZAdd(ctx, riskIPInsightIndexKey, redis.Z{
		Score:  float64(now.Unix()),
		Member: normalizedIP,
	})

	if event != nil {
		event.IP = normalizedIP
		if event.OccurredAt == "" {
			event.OccurredAt = now.Format(time.RFC3339)
		}
		if event.OccurredAtUnix == 0 {
			event.OccurredAtUnix = now.Unix()
		}

		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}

		pipe.LPush(ctx, eventsKey, raw)
		pipe.LTrim(ctx, eventsKey, 0, riskIPInsightEventLimit-1)
		pipe.Expire(ctx, eventsKey, riskIPInsightTTL)
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (s *RiskService) loadRiskIPSummary(ctx context.Context, ip string) (*RiskIPSummary, bool, error) {
	profile, err := s.Rdb.HGetAll(ctx, riskIPInsightProfileKey(ip)).Result()
	if err != nil {
		return nil, false, err
	}

	blacklisted, blacklistReason, blacklistExpireAt, err := s.currentBlacklistState(ctx, ip)
	if err != nil {
		return nil, false, err
	}
	loginFailCount, loginFailExpireAt, err := s.currentLoginFailState(ctx, ip)
	if err != nil {
		return nil, false, err
	}

	if len(profile) == 0 && !blacklisted && loginFailCount == 0 {
		return nil, false, nil
	}

	summary := &RiskIPSummary{
		IP:                       ip,
		FirstSeenAt:              unixString(parseInt64(profile["first_seen_at"])),
		LastSeenAt:               unixString(parseInt64(profile["last_seen_at"])),
		LastScene:                profile["last_scene"],
		LastAction:               profile["last_action"],
		LastReason:               profile["last_reason"],
		LastReqID:                profile["last_req_id"],
		LatestUserIDMasked:       profile["latest_user_id_masked"],
		TotalChecks:              parseInt64(profile["total_checks"]),
		TotalRejects:             parseInt64(profile["total_rejects"]),
		TotalVerifies:            parseInt64(profile["total_verifies"]),
		TotalReportSuccess:       parseInt64(profile["total_report_success"]),
		TotalReportFailure:       parseInt64(profile["total_report_failure"]),
		TotalBlacklistHits:       parseInt64(profile["total_blacklist_hits"]),
		CurrentLoginFailCount:    loginFailCount,
		CurrentLoginFailExpireAt: loginFailExpireAt,
		Blacklisted:              blacklisted,
		BlacklistReason:          blacklistReason,
		BlacklistExpireAt:        blacklistExpireAt,
	}
	summary.Flags, summary.Severity = s.buildRiskFlags(summary)

	return summary, true, nil
}

func (s *RiskService) currentBlacklistState(ctx context.Context, ip string) (bool, string, string, error) {
	key := fmt.Sprintf("risk:blacklist:ip:%s", ip)
	reason, err := s.Rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, "", "", nil
		}
		return false, "", "", err
	}

	ttl, err := s.Rdb.TTL(ctx, key).Result()
	if err != nil {
		return false, "", "", err
	}

	return true, reason, ttlExpiryString(ttl), nil
}

func (s *RiskService) currentLoginFailState(ctx context.Context, ip string) (int64, string, error) {
	key := riskIPInsightLoginFailCounterKey(ip)
	val, err := s.Rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, "", nil
		}
		return 0, "", err
	}

	count, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, "", err
	}

	ttl, err := s.Rdb.TTL(ctx, key).Result()
	if err != nil {
		return 0, "", err
	}

	return count, ttlExpiryString(ttl), nil
}

func (s *RiskService) buildRiskFlags(summary *RiskIPSummary) ([]string, string) {
	flags := make([]string, 0, 4)
	if summary.Blacklisted {
		flags = append(flags, "blacklisted")
	}
	if summary.CurrentLoginFailCount > 0 {
		flags = append(flags, "login_fail_active")
	}
	if summary.TotalRejects > 0 {
		flags = append(flags, "reject_observed")
	}
	if summary.TotalVerifies > 0 {
		flags = append(flags, "verify_observed")
	}

	threshold := int64(1)
	if s.Config != nil && s.Config.Login.MaxFailCount > 0 {
		threshold = int64(s.Config.Login.MaxFailCount)
	}

	severity := "low"
	switch {
	case summary.Blacklisted:
		severity = "critical"
	case summary.CurrentLoginFailCount >= threshold:
		severity = "high"
	case summary.TotalRejects > 0 || summary.CurrentLoginFailCount > 0:
		severity = "medium"
	case summary.TotalVerifies > 0 || summary.TotalBlacklistHits > 0:
		severity = "medium"
	}

	return flags, severity
}

func riskIPInsightProfileKey(ip string) string {
	return riskIPInsightProfilePrefix + ip
}

func riskIPInsightEventsKey(ip string) string {
	return riskIPInsightEventsPrefix + ip
}

func riskIPInsightLoginFailCounterKey(ip string) string {
	return riskIPInsightLoginFailKey + ip
}

func normalizeRiskIP(ip string) (string, error) {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return "", ErrInvalidRiskIP
	}
	parsed := net.ParseIP(trimmed)
	if parsed == nil {
		return "", ErrInvalidRiskIP
	}
	return parsed.String(), nil
}

func parseInt64(raw string) int64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func unixString(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.Unix(value, 0).UTC().Format(time.RFC3339)
}

func ttlExpiryString(ttl time.Duration) string {
	if ttl <= 0 {
		return ""
	}
	return time.Now().UTC().Add(ttl).Format(time.RFC3339)
}

func maskUserID(userID string) string {
	if userID == "" {
		return ""
	}
	if len(userID) <= 4 {
		return userID
	}
	return userID[:2] + strings.Repeat("*", len(userID)-4) + userID[len(userID)-2:]
}

func actionLabel(action pb.Action) string {
	switch action {
	case pb.Action_ACTION_REJECT:
		return "reject"
	case pb.Action_ACTION_VERIFY:
		return "verify"
	default:
		return "pass"
	}
}

func sceneLabel(scene pb.Scene) string {
	switch scene {
	case pb.Scene_SCENE_LOGIN:
		return "login"
	case pb.Scene_SCENE_REGISTER:
		return "register"
	case pb.Scene_SCENE_PAYMENT:
		return "payment"
	default:
		return "unknown"
	}
}

func clampInt(value int, defaultValue int, maxValue int) int {
	switch {
	case value <= 0:
		return defaultValue
	case value > maxValue:
		return maxValue
	default:
		return value
	}
}

func newRiskIPEventID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func boolPtr(value bool) *bool {
	return &value
}

func (s *RiskService) recordRiskIPLoginFailCounter(ctx context.Context, ip string) (int64, error) {
	expireSeconds := s.Config.Login.FailCountExpireMinutes * int(time.Minute/time.Second)
	if expireSeconds <= 0 {
		return 0, fmt.Errorf("invalid fail_count_expire_minutes: %d", s.Config.Login.FailCountExpireMinutes)
	}

	return loginFailCountLua.Run(ctx, s.Rdb, []string{riskIPInsightLoginFailCounterKey(ip)}, expireSeconds).Int64()
}
