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
	RoleStudent = 1
	RoleTeacher = 2
	RoleAdmin   = 3

	principalContextKey = "risk_admin_principal"

	standardUserIDHeader    = "user_id"
	standardUserRolesHeader = "user_roles"
	gatewayUserIDHeader     = "X-User-Id"
	gatewayUserRolesHeader  = "X-User-Roles"
)

// RequestPrincipal 表示网关注入的经过身份验证的用户身份信息。
// 它包含从请求头解析出的用户ID和角色列表。
type RequestPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

// RiskAdminAuthMiddleware 是风控后台鉴权中间件。
// 该中间件信任由网关注入的身份信息请求头，且只负责在本地校验用户身份是否存在以及角色权限是否匹配允许的角色列表 (allowedRoles)
func RiskAdminAuthMiddleware(logger *zap.Logger, allowedRoles ...int) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		principal, err := parseRequestPrincipal(c)
		if err != nil {
			logger.Warn("risk admin auth rejected: invalid gateway principal",
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

// parseRequestPrincipal 从HTTP请求上下文中提取并解析网关传递的用户凭证(userID和roles)。
// 如果上下文为空、请求头缺失或格式非法，将返回相应的错误。
func parseRequestPrincipal(c *gin.Context) (*RequestPrincipal, error) {
	if c == nil {
		return nil, errInvalidPrincipal
	}

	userID, err := parseUserID(c.Request.Header)
	if err != nil {
		return nil, err
	}

	roles, err := parseRoles(c.Request.Header)
	if err != nil {
		return nil, errInvalidPrincipal
	}

	return &RequestPrincipal{
		UserID: userID,
		Roles:  roles,
	}, nil
	// parseUserID 解析请求头中的用户ID信息。
	// 支持读取标准的 user_id 头和网关规范的 X-User-Id 头。
	// 若同时解析到两个不同头中的userID不一致，将认定为非法请求。
}

func parseUserID(headers http.Header) (string, error) {
	if headers == nil {
		return "", errInvalidPrincipal
	}

	var userID string
	for _, headerName := range []string{standardUserIDHeader, gatewayUserIDHeader} {
		value := strings.TrimSpace(headers.Get(headerName))
		if value == "" {
			continue
		}
		if userID == "" {
			userID = value
			continue
		}
		if value != userID {
			return "", errInvalidPrincipal
		}
	}

	if userID == "" {
		return "", errInvalidPrincipal
	}

	// parseRoles 解析请求头中的角色列表。
	// 支持标准和网关两种请求头(user_roles / X-User-Roles)。同时处理这两者时，必须保证角色集合一致，否则报错。
	return userID, nil
}

func parseRoles(headers http.Header) ([]int, error) {
	if headers == nil {
		return nil, errInvalidPrincipal
	}

	var roles []int
	var hasRoles bool
	for _, headerName := range []string{standardUserRolesHeader, gatewayUserRolesHeader} {
		parsed, err := parseRoleValues(headers.Values(headerName))
		if err != nil {
			return nil, errInvalidPrincipal
		}
		if len(parsed) == 0 {
			continue
		}
		if !hasRoles {
			roles = parsed
			hasRoles = true
			continue
		}
		if !sameRoleSet(roles, parsed) {
			return nil, errInvalidPrincipal
		}
	}

	if !hasRoles {
		return nil, errInvalidPrincipal
	}

	return roles, nil
}

// sameRoleSet 比较两个角色集合内包含的元素是否一致(忽略顺序)。
func sameRoleSet(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}

	seen := make(map[int]struct{}, len(left))
	for _, role := range left {
		seen[role] = struct{}{}
	}

	for _, role := range right {
		if _, ok := seen[role]; !ok {
			return false
		}
	}

	return true
}

// HasAnyRole 校验当前用户实体是否至少具有指定的可接受角色列表(roles)中的任意一个角色。
// 如果没有任何匹配，或实体本身角色为空，则返回 false。
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
	// parseRoleValues 对获取到的角色字符串列表进行解析。
	// 支持对 JSON 数组格式（以 '[' 开头）以及逗号分隔字符串（如 "1,2,3"）的解析。
	// 解析时自动去重并过滤无效值。
}

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

	// parseJSONRoles 解析JSON形式的角色列表。
	// 支持 "[1, 2]" 以及 '["1", "2"]' 格式。
	return roles, nil
}

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
		// parseDelimitedRoles 解析基于逗号分隔的角色字符串。
		// 支持如 "1,2,3" 或 '"1","2"' 这种格式。
	}

	return roles, nil
}

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
