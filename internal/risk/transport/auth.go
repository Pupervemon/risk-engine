package transport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// 角色枚举与上游透传的权限标识保持一致，用于做最小权限判断。
	RoleStudent = 1
	RoleTeacher = 2
	RoleAdmin   = 3
)

// principalContextKey 用于把解析后的身份信息放入 Gin 上下文，供后续 handler 直接读取。
const principalContextKey = "risk_admin_principal"

// RequestPrincipal 表示一次请求中携带的最小身份信息，只保留鉴权所需字段。
type RequestPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

// RiskAdminAuthMiddleware 从请求头中提取身份并进行角色校验，
// 通过后将 principal 写入上下文，失败则直接返回 401 或 403。
func RiskAdminAuthMiddleware(logger *zap.Logger, allowedRoles ...int) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := parseRequestPrincipal(c)
		if err != nil {
			logger.Warn("risk admin auth rejected: invalid principal",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "UNAUTHORIZED"})
			return
		}

		if !principal.HasAnyRole(allowedRoles...) {
			logger.Warn("risk admin auth rejected: insufficient role",
				zap.String("user_id", principal.UserID),
				zap.Ints("user_roles", principal.Roles),
				zap.String("path", c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "FORBIDDEN"})
			return
		}

		c.Set(principalContextKey, principal)
		c.Next()
	}
}

// parseRequestPrincipal 兼容多种请求头命名，从中提取用户标识和角色列表。
// 只要任一关键字段无法解析，就返回非法身份错误，避免错误放行。
func parseRequestPrincipal(c *gin.Context) (*RequestPrincipal, error) {
	userID := strings.TrimSpace(firstNonEmptyHeaderValue(c, "user_id", "x-user-id", "x_user_id"))
	if userID == "" {
		return nil, errInvalidPrincipal
	}

	roleValues := headerValues(c, "user_roles", "x-user-roles", "x_user_roles")
	roles, err := parseRoleValues(roleValues)
	if err != nil || len(roles) == 0 {
		return nil, errInvalidPrincipal
	}

	return &RequestPrincipal{
		UserID: userID,
		Roles:  roles,
	}, nil
}

// HasAnyRole 判断当前请求是否至少拥有一个允许的角色。
// 先构建集合再匹配，避免重复角色带来的多余遍历。
func (p *RequestPrincipal) HasAnyRole(roles ...int) bool {
	if p == nil {
		return false
	}

	current := make(map[int]struct{}, len(p.Roles))
	for _, role := range p.Roles {
		current[role] = struct{}{}
	}

	for _, role := range roles {
		if _, ok := current[role]; ok {
			return true
		}
	}
	return false
}

// headerValues 按优先级读取多个候选头名，返回第一个包含值的结果。
// 用于兼容不同网关或调用方可能使用的字段名。
func headerValues(c *gin.Context, names ...string) []string {
	for _, name := range names {
		values := c.Request.Header.Values(name)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

// firstNonEmptyHeaderValue 在候选头值中找到第一个非空项，并去除两端空白。
func firstNonEmptyHeaderValue(c *gin.Context, names ...string) string {
	values := headerValues(c, names...)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// parseRoleValues 把角色头解析为去重后的整数数组。
// 支持 JSON 数组和逗号分隔字符串两种常见格式。
func parseRoleValues(values []string) ([]int, error) {
	roles := make([]int, 0, len(values))
	seen := map[int]struct{}{}

	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		var parsed []int
		var err error
		if strings.HasPrefix(trimmed, "[") {
			parsed, err = parseJSONRoles(trimmed)
		} else {
			parsed, err = parseDelimitedRoles(trimmed)
		}
		if err != nil {
			return nil, err
		}

		for _, role := range parsed {
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}

	return roles, nil
}

// parseJSONRoles 解析 JSON 格式的角色列表。
// 先尝试整数数组，再兼容字符串数组，降低上游序列化差异带来的影响。
func parseJSONRoles(raw string) ([]int, error) {
	var ints []int
	if err := json.Unmarshal([]byte(raw), &ints); err == nil {
		return ints, nil
	}

	var stringsValue []string
	if err := json.Unmarshal([]byte(raw), &stringsValue); err != nil {
		return nil, errInvalidPrincipal
	}

	roles := make([]int, 0, len(stringsValue))
	for _, value := range stringsValue {
		role, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, errInvalidPrincipal
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// parseDelimitedRoles 解析逗号分隔的角色列表，并兼容带引号和空格的输入。
func parseDelimitedRoles(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	roles := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.Trim(part, "\""))
		if trimmed == "" {
			continue
		}

		role, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, errInvalidPrincipal
		}
		roles = append(roles, role)
	}

	return roles, nil
}
