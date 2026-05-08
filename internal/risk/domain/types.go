package domain

import "strings"

type Scene string

const (
	SceneUnknown  Scene = "unknown"
	SceneLogin    Scene = "login"
	SceneRegister Scene = "register"
	ScenePayment  Scene = "payment"
)

type Action string

const (
	ActionPass   Action = "pass"
	ActionReject Action = "reject"
	ActionVerify Action = "verify"
)

type BlacklistType string

const (
	BlacklistTypeIP      BlacklistType = "ip"
	BlacklistTypeUserID  BlacklistType = "uid"
	BlacklistTypeUnknown BlacklistType = "unknown"
)

type BlacklistEntry struct {
	Type     BlacklistType
	Value    string
	Reason   string
	ExpireAt int64
}

type RateLimitRule struct {
	Limit         int
	WindowSeconds int
}

type RateLimitResult struct {
	Count    int64
	Exceeded bool
}

type LoginFailureTargetType string

const (
	LoginFailureTargetUserID LoginFailureTargetType = "userId"
	LoginFailureTargetIP     LoginFailureTargetType = "ip"
)

type LoginFailureTarget struct {
	Type  LoginFailureTargetType
	Value string
}

func ActionLabel(action Action) string {
	if action == "" {
		return string(ActionPass)
	}
	return string(action)
}

func SceneLabel(scene Scene) string {
	if scene == "" {
		return string(SceneUnknown)
	}
	return string(scene)
}

func IsBlacklistReason(reason string) bool {
	return strings.HasPrefix(reason, "BLACKLIST_")
}
