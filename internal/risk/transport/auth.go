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
)

const principalContextKey = "risk_admin_principal"

type RequestPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

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

func headerValues(c *gin.Context, names ...string) []string {
	for _, name := range names {
		values := c.Request.Header.Values(name)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

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
