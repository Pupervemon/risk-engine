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

// RequestPrincipal is the authenticated identity injected by the gateway.
type RequestPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

// RiskAdminAuthMiddleware trusts the gateway-injected identity headers and only
// performs local presence and role checks.
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
