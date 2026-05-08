package http

// HealthComponentDoc documents the health status of a single dependency.
type HealthComponentDoc struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthResponseDoc documents the health-check response payload.
type HealthResponseDoc struct {
	Status     string                        `json:"status"`
	Components map[string]HealthComponentDoc `json:"components,omitempty"`
	Timestamp  string                        `json:"timestamp"`
}

// RiskIPSummaryDoc documents the aggregated risk profile of an IP.
type RiskIPSummaryDoc struct {
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

// RiskIPDetailDoc documents a single IP insight response.
type RiskIPDetailDoc struct {
	RiskIPSummaryDoc
	EventCount int64 `json:"event_count"`
}

// RiskIPListResponseDoc documents the paginated IP insight list response.
type RiskIPListResponseDoc struct {
	Items   []RiskIPSummaryDoc `json:"items"`
	Offset  int                `json:"offset"`
	Limit   int                `json:"limit"`
	Total   int64              `json:"total"`
	HasMore bool               `json:"has_more"`
}

// RiskIPEventDoc documents a single risk IP event.
type RiskIPEventDoc struct {
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

// RiskIPEventsResponseDoc documents the paginated event history response.
type RiskIPEventsResponseDoc struct {
	IP      string           `json:"ip"`
	Items   []RiskIPEventDoc `json:"items"`
	Offset  int              `json:"offset"`
	Limit   int              `json:"limit"`
	Total   int64            `json:"total"`
	HasMore bool             `json:"has_more"`
}
