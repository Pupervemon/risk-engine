package redis

import (
	"fmt"

	"github.com/Pupervemon/risk-engine/internal/risk/domain"
)

const (
	riskIPInsightIndexKey      = "risk:insight:ip:index:last_seen"
	riskIPInsightProfilePrefix = "risk:insight:ip:profile:"
	riskIPInsightEventsPrefix  = "risk:insight:ip:events:"
	riskIPInsightLoginFailKey  = "risk:insight:ip:login_fail:"
)

func blacklistKey(entryType domain.BlacklistType, value string) string {
	return fmt.Sprintf("risk:blacklist:%s:%s", entryType, value)
}

func ipRateLimitKey(ip string) string {
	return fmt.Sprintf("risk:rate:ip:%s", ip)
}

func userRateLimitKey(userID string, scope string) string {
	return fmt.Sprintf("risk:rate:user:%s:%s", userID, scope)
}

func loginFailCountKey(target domain.LoginFailureTarget) string {
	switch target.Type {
	case domain.LoginFailureTargetUserID:
		return fmt.Sprintf("risk:fail_count:login:uid:%s", target.Value)
	case domain.LoginFailureTargetIP:
		return fmt.Sprintf("risk:fail_count:login:ip:%s", target.Value)
	default:
		return fmt.Sprintf("risk:fail_count:login:%s:%s", target.Type, target.Value)
	}
}

func loginFailCountKeyByUserID(userID string) string {
	return fmt.Sprintf("risk:fail_count:login:uid:%s", userID)
}

func loginFailCountKeyByIP(ip string) string {
	return fmt.Sprintf("risk:fail_count:login:ip:%s", ip)
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
