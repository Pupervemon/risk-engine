package domain

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

type RiskCheckInsight struct {
	ReqID  string
	Scene  Scene
	IP     string
	UserID string
	Action Action
	Reason string
}

type RiskReportInsight struct {
	ReqID          string
	Scene          Scene
	IP             string
	UserID         string
	IsSuccess      bool
	Received       bool
	LoginFailCount int64
}

type RiskBlacklistInsight struct {
	Type     BlacklistType
	Value    string
	Reason   string
	ExpireAt int64
}

func BuildRiskFlags(summary *RiskIPSummary, loginFailThreshold int64) ([]string, string) {
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
	if loginFailThreshold > 0 {
		threshold = loginFailThreshold
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
