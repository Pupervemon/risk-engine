package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	RoleAdmin = 3

	captchaAdminPrincipalContextKey = "captcha_admin_principal"
	captchaGatewayUserIDHeader      = "X-User-Id"
	captchaGatewayUserRolesHeader   = "X-User-Roles"
)

// AdminPrincipal is the gateway-injected identity used for captcha admin endpoints.
type AdminPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

// NewAdminAuthMiddleware trusts the gateway-injected principal headers and only
// performs local header presence and role checks.
func NewAdminAuthMiddleware(logger *zap.Logger, allowedRoles ...int) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		principal, err := parseAdminPrincipal(c)
		if err != nil {
			logger.Warn("captcha admin auth rejected: invalid gateway principal",
				zap.Error(err),
				zap.String("path", c.Request.URL.Path))
			writeJSON(c, http.StatusUnauthorized, errorResponse{
				Error:  "ADMIN_UNAUTHORIZED",
				Reason: "INVALID_GATEWAY_PRINCIPAL",
			})
			c.Abort()
			return
		}

		if !principal.HasAnyRole(allowedRoles...) {
			logger.Warn("captcha admin auth rejected: insufficient role",
				zap.String("user_id", principal.UserID),
				zap.Ints("user_roles", principal.Roles),
				zap.String("path", c.Request.URL.Path))
			writeJSON(c, http.StatusForbidden, errorResponse{
				Error:  "ADMIN_FORBIDDEN",
				Reason: "INSUFFICIENT_ROLE",
			})
			c.Abort()
			return
		}

		c.Set(captchaAdminPrincipalContextKey, principal)
		c.Next()
	}
}

func parseAdminPrincipal(c *gin.Context) (*AdminPrincipal, error) {
	if c == nil {
		return nil, errInvalidAdminPrincipal
	}

	userID := strings.TrimSpace(c.GetHeader(captchaGatewayUserIDHeader))
	if userID == "" {
		return nil, errInvalidAdminPrincipal
	}

	roles, err := parseAdminRoleValues(c.Request.Header.Values(captchaGatewayUserRolesHeader))
	if err != nil || len(roles) == 0 {
		return nil, errInvalidAdminPrincipal
	}

	return &AdminPrincipal{
		UserID: userID,
		Roles:  roles,
	}, nil
}

func (p *AdminPrincipal) HasAnyRole(roles ...int) bool {
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

func parseAdminRoleValues(values []string) ([]int, error) {
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
			parsed, err = parseAdminJSONRoles(trimmed)
		} else {
			parsed, err = parseAdminDelimitedRoles(trimmed)
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

func parseAdminJSONRoles(raw string) ([]int, error) {
	var ints []int
	if err := json.Unmarshal([]byte(raw), &ints); err == nil {
		return ints, nil
	}

	var stringsValue []string
	if err := json.Unmarshal([]byte(raw), &stringsValue); err != nil {
		return nil, errInvalidAdminPrincipal
	}

	roles := make([]int, 0, len(stringsValue))
	for _, value := range stringsValue {
		role, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, errInvalidAdminPrincipal
		}
		roles = append(roles, role)
	}

	return roles, nil
}

func parseAdminDelimitedRoles(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	roles := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.Trim(part, "\""))
		if trimmed == "" {
			continue
		}

		role, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, errInvalidAdminPrincipal
		}
		roles = append(roles, role)
	}

	return roles, nil
}
