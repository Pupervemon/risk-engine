package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	riskIPInsightTTL         = 30 * 24 * time.Hour
	riskIPInsightEventLimit  = 200
	riskIPListDefaultLimit   = 20
	riskIPListMaxLimit       = 100
	riskIPEventsDefaultLimit = 50
	riskIPEventsMaxLimit     = 200
)

type RiskInsightRepository struct {
	Rdb                    *goredis.Client
	LoginFailExpireMinutes int
	Logger                 *zap.Logger
}

func NewRiskInsightRepository(rdb *goredis.Client, loginFailExpireMinutes int, logger *zap.Logger) *RiskInsightRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RiskInsightRepository{
		Rdb:                    rdb,
		LoginFailExpireMinutes: loginFailExpireMinutes,
		Logger:                 logger,
	}
}

func (r *RiskInsightRepository) RecordCheck(ctx context.Context, insight domain.RiskCheckInsight) error {
	if insight.IP == "" {
		return nil
	}

	fields := map[string]interface{}{
		"last_scene":  domain.SceneLabel(insight.Scene),
		"last_action": domain.ActionLabel(insight.Action),
		"last_reason": insight.Reason,
	}
	if insight.ReqID != "" {
		fields["last_req_id"] = insight.ReqID
	}
	if masked := maskUserID(insight.UserID); masked != "" {
		fields["latest_user_id_masked"] = masked
	}

	increments := map[string]int64{
		"total_checks": 1,
	}
	switch insight.Action {
	case domain.ActionReject:
		increments["total_rejects"] = 1
	case domain.ActionVerify:
		increments["total_verifies"] = 1
	}
	if domain.IsBlacklistReason(insight.Reason) {
		increments["total_blacklist_hits"] = 1
	}

	now := time.Now().UTC()
	event := domain.RiskIPEvent{
		EventID:        newRiskIPEventID("check"),
		EventType:      "check",
		IP:             insight.IP,
		Scene:          domain.SceneLabel(insight.Scene),
		Action:         domain.ActionLabel(insight.Action),
		Reason:         insight.Reason,
		ReqID:          insight.ReqID,
		UserIDMasked:   maskUserID(insight.UserID),
		OccurredAt:     now.Format(time.RFC3339),
		OccurredAtUnix: now.Unix(),
	}

	return r.writeRiskIPInsight(ctx, insight.IP, fields, increments, &event)
}

func (r *RiskInsightRepository) RecordReportEvent(ctx context.Context, insight domain.RiskReportInsight) error {
	if insight.IP == "" {
		return nil
	}

	insightFailCount := insight.LoginFailCount
	if insight.IsSuccess {
		if err := r.Rdb.Del(ctx, riskIPInsightLoginFailCounterKey(insight.IP)).Err(); err != nil && !errors.Is(err, goredis.Nil) {
			return err
		}
		insightFailCount = 0
	} else {
		count, err := r.recordRiskIPLoginFailCounter(ctx, insight.IP)
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
	if insight.IsSuccess {
		action = "success"
		reason = "LOGIN_SUCCESS"
		increments = map[string]int64{
			"total_report_success": 1,
		}
	}

	fields := map[string]interface{}{
		"last_scene":  domain.SceneLabel(insight.Scene),
		"last_action": "report_" + action,
		"last_reason": reason,
	}
	if insight.ReqID != "" {
		fields["last_req_id"] = insight.ReqID
	}
	if masked := maskUserID(insight.UserID); masked != "" {
		fields["latest_user_id_masked"] = masked
	}

	now := time.Now().UTC()
	event := domain.RiskIPEvent{
		EventID:        newRiskIPEventID("report"),
		EventType:      "report_" + action,
		IP:             insight.IP,
		Scene:          domain.SceneLabel(insight.Scene),
		Action:         action,
		Reason:         reason,
		ReqID:          insight.ReqID,
		UserIDMasked:   maskUserID(insight.UserID),
		Received:       boolPtr(insight.Received),
		LoginFailCount: insightFailCount,
		OccurredAt:     now.Format(time.RFC3339),
		OccurredAtUnix: now.Unix(),
	}

	return r.writeRiskIPInsight(ctx, insight.IP, fields, increments, &event)
}

func (r *RiskInsightRepository) RecordBlacklist(ctx context.Context, insight domain.RiskBlacklistInsight) error {
	if insight.Type != domain.BlacklistTypeIP || insight.Value == "" {
		return nil
	}

	expireAt := ""
	if insight.ExpireAt > 0 {
		expireAt = time.Unix(insight.ExpireAt, 0).UTC().Format(time.RFC3339)
	}

	fields := map[string]interface{}{
		"last_action": "blacklist_added",
		"last_reason": insight.Reason,
	}

	now := time.Now().UTC()
	event := domain.RiskIPEvent{
		EventID:            newRiskIPEventID("blacklist"),
		EventType:          "blacklist_added",
		IP:                 insight.Value,
		Action:             "blacklist_added",
		Reason:             insight.Reason,
		OccurredAt:         now.Format(time.RFC3339),
		OccurredAtUnix:     now.Unix(),
		BlacklistExpireAt:  expireAt,
		CurrentBlacklisted: true,
	}

	return r.writeRiskIPInsight(ctx, insight.Value, fields, nil, &event)
}

func (r *RiskInsightRepository) ListRiskIPs(ctx context.Context, query ports.RiskIPListQuery, loginFailThreshold int64) (*ports.RiskIPListResponse, error) {
	limit := clampInt(query.Limit, riskIPListDefaultLimit, riskIPListMaxLimit)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	search := strings.TrimSpace(query.Search)
	if search != "" {
		detail, err := r.GetRiskIP(ctx, search, loginFailThreshold)
		if err != nil {
			if errors.Is(err, domain.ErrRiskIPNotFound) {
				return &ports.RiskIPListResponse{
					Items:   []domain.RiskIPSummary{},
					Offset:  0,
					Limit:   limit,
					Total:   0,
					HasMore: false,
				}, nil
			}
			return nil, err
		}

		return &ports.RiskIPListResponse{
			Items:   []domain.RiskIPSummary{detail.RiskIPSummary},
			Offset:  0,
			Limit:   limit,
			Total:   1,
			HasMore: false,
		}, nil
	}

	total, err := r.Rdb.ZCard(ctx, riskIPInsightIndexKey).Result()
	if err != nil {
		return nil, err
	}

	members, err := r.Rdb.ZRevRange(ctx, riskIPInsightIndexKey, int64(offset), int64(offset+limit)).Result()
	if err != nil {
		return nil, err
	}

	hasMore := len(members) > limit
	if hasMore {
		members = members[:limit]
	}

	items := make([]domain.RiskIPSummary, 0, len(members))
	for _, ip := range members {
		detail, err := r.GetRiskIP(ctx, ip, loginFailThreshold)
		if err != nil {
			if errors.Is(err, domain.ErrRiskIPNotFound) {
				_ = r.Rdb.ZRem(ctx, riskIPInsightIndexKey, ip).Err()
				continue
			}
			return nil, err
		}
		items = append(items, detail.RiskIPSummary)
	}

	return &ports.RiskIPListResponse{
		Items:   items,
		Offset:  offset,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (r *RiskInsightRepository) GetRiskIP(ctx context.Context, ip string, loginFailThreshold int64) (*domain.RiskIPDetail, error) {
	normalizedIP, err := normalizeRiskIP(ip)
	if err != nil {
		return nil, err
	}

	summary, found, err := r.loadRiskIPSummary(ctx, normalizedIP, loginFailThreshold)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.ErrRiskIPNotFound
	}

	eventCount, err := r.Rdb.LLen(ctx, riskIPInsightEventsKey(normalizedIP)).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, err
	}

	return &domain.RiskIPDetail{
		RiskIPSummary: *summary,
		EventCount:    eventCount,
	}, nil
}

func (r *RiskInsightRepository) ListRiskIPEvents(ctx context.Context, ip string, query ports.RiskIPEventsQuery) (*ports.RiskIPEventsResponse, error) {
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
	total, err := r.Rdb.LLen(ctx, eventsKey).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, err
	}

	if total == 0 {
		if _, err := r.GetRiskIP(ctx, normalizedIP, 0); err != nil {
			return nil, err
		}
		return &ports.RiskIPEventsResponse{
			IP:      normalizedIP,
			Items:   []domain.RiskIPEvent{},
			Offset:  offset,
			Limit:   limit,
			Total:   0,
			HasMore: false,
		}, nil
	}

	values, err := r.Rdb.LRange(ctx, eventsKey, int64(offset), int64(offset+limit)).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, err
	}

	hasMore := len(values) > limit
	if hasMore {
		values = values[:limit]
	}

	items := make([]domain.RiskIPEvent, 0, len(values))
	for _, raw := range values {
		var event domain.RiskIPEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			r.Logger.Warn("failed to decode risk ip event", zap.Error(err), zap.String("ip", normalizedIP))
			continue
		}
		items = append(items, event)
	}

	return &ports.RiskIPEventsResponse{
		IP:      normalizedIP,
		Items:   items,
		Offset:  offset,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (r *RiskInsightRepository) writeRiskIPInsight(ctx context.Context, ip string, fields map[string]interface{}, increments map[string]int64, event *domain.RiskIPEvent) error {
	normalizedIP, err := normalizeRiskIP(ip)
	if err != nil {
		return nil
	}

	now := time.Now().UTC()
	profileKey := riskIPInsightProfileKey(normalizedIP)
	eventsKey := riskIPInsightEventsKey(normalizedIP)

	pipe := r.Rdb.TxPipeline()
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
	pipe.ZAdd(ctx, riskIPInsightIndexKey, goredis.Z{
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

func (r *RiskInsightRepository) loadRiskIPSummary(ctx context.Context, ip string, loginFailThreshold int64) (*domain.RiskIPSummary, bool, error) {
	profile, err := r.Rdb.HGetAll(ctx, riskIPInsightProfileKey(ip)).Result()
	if err != nil {
		return nil, false, err
	}

	blacklisted, blacklistReason, blacklistExpireAt, err := r.currentBlacklistState(ctx, ip)
	if err != nil {
		return nil, false, err
	}
	loginFailCount, loginFailExpireAt, err := r.currentLoginFailState(ctx, ip)
	if err != nil {
		return nil, false, err
	}

	if len(profile) == 0 && !blacklisted && loginFailCount == 0 {
		return nil, false, nil
	}

	summary := &domain.RiskIPSummary{
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
	summary.Flags, summary.Severity = domain.BuildRiskFlags(summary, loginFailThreshold)

	return summary, true, nil
}

func (r *RiskInsightRepository) currentBlacklistState(ctx context.Context, ip string) (bool, string, string, error) {
	key := blacklistKey(domain.BlacklistTypeIP, ip)
	reason, err := r.Rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return false, "", "", nil
		}
		return false, "", "", err
	}

	ttl, err := r.Rdb.TTL(ctx, key).Result()
	if err != nil {
		return false, "", "", err
	}

	return true, reason, ttlExpiryString(ttl), nil
}

func (r *RiskInsightRepository) currentLoginFailState(ctx context.Context, ip string) (int64, string, error) {
	key := riskIPInsightLoginFailCounterKey(ip)
	val, err := r.Rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return 0, "", nil
		}
		return 0, "", err
	}

	count, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, "", err
	}

	ttl, err := r.Rdb.TTL(ctx, key).Result()
	if err != nil {
		return 0, "", err
	}

	return count, ttlExpiryString(ttl), nil
}

func (r *RiskInsightRepository) recordRiskIPLoginFailCounter(ctx context.Context, ip string) (int64, error) {
	expireSeconds := r.LoginFailExpireMinutes * int(time.Minute/time.Second)
	if expireSeconds <= 0 {
		return 0, fmt.Errorf("invalid fail_count_expire_minutes: %d", r.LoginFailExpireMinutes)
	}

	return loginFailCountLua.Run(ctx, r.Rdb, []string{riskIPInsightLoginFailCounterKey(ip)}, expireSeconds).Int64()
}

func normalizeRiskIP(ip string) (string, error) {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return "", domain.ErrInvalidRiskIP
	}
	parsed := net.ParseIP(trimmed)
	if parsed == nil {
		return "", domain.ErrInvalidRiskIP
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
