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
	// RoleAdmin 是网关用于标记验证码管理员的角色值。
	RoleAdmin = 3

	// captchaAdminPrincipalContextKey 用于把解析后的网关身份放入 Gin 上下文。
	captchaAdminPrincipalContextKey = "captcha_admin_principal"
	// captchaGatewayStandardUserIDHeader 是下游通用解析器使用的用户 ID 头。
	captchaGatewayStandardUserIDHeader = "user_id"
	// captchaGatewayStandardUserRolesHeader 是下游通用解析器使用的角色头。
	captchaGatewayStandardUserRolesHeader = "user_roles"
	// captchaGatewayUserIDHeader 保存网关注入的已认证用户 ID。
	captchaGatewayUserIDHeader = "X-User-Id"
	// captchaGatewayUserRolesHeader 保存网关注入的调用方角色列表。
	captchaGatewayUserRolesHeader = "X-User-Roles"
)

// AdminPrincipal 表示由受信任网关注入的身份信息。
// 这里不会做完整认证，只会校验网关是否提供了可用的身份，以及该身份是否具备所需角色。
type AdminPrincipal struct {
	UserID string `json:"user_id"`
	Roles  []int  `json:"roles"`
}

// NewAdminAuthMiddleware 构建一个 Gin 中间件，它接受网关注入的身份头，
// 校验其格式，然后执行允许角色检查。
//
// 该中间件默认上游网关已经完成认证，这里只负责对头部和角色列表做本地校验。
func NewAdminAuthMiddleware(logger *zap.Logger, allowedRoles ...int) gin.HandlerFunc {
	if logger == nil {
		// 即使调用方没有传入 logger，也要保证中间件可以安全使用。
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		// 从网关注入的请求头中解析出身份信息。
		principal, err := parseAdminPrincipal(c)
		if err != nil {
			// 拒绝没有有效网关身份信息的请求。
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

		// 执行接口的授权策略。
		// 如果身份不包含任何一个允许角色，就直接返回禁止访问。
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

		// 将解析后的身份保存到上下文中，供后续处理器使用，然后继续请求链。
		c.Set(captchaAdminPrincipalContextKey, principal)
		c.Next()
	}
}

// parseAdminPrincipal 读取网关提供的请求头，并将其转换为结构化身份。
//
// 必填项：
// - `user_id` 或 `X-User-Id` 必须存在且非空。
// - `user_roles` 或 `X-User-Roles` 至少要能解析出一个角色。
func parseAdminPrincipal(c *gin.Context) (*AdminPrincipal, error) {
	if c == nil {
		// 上下文为空时，中间件无法安全读取请求头。
		return nil, errInvalidAdminPrincipal
	}

	userID, err := parseAdminUserID(c.Request.Header)
	if err != nil {
		return nil, err
	}

	roles, err := parseAdminRoles(c.Request.Header)
	if err != nil {
		return nil, errInvalidAdminPrincipal
	}

	// 构造后续处理器用于授权判断的身份对象。
	return &AdminPrincipal{
		UserID: userID,
		Roles:  roles,
	}, nil
}

// parseAdminUserID 会在兼容的新旧请求头别名中解析用户 ID。
// 如果多个别名同时出现但值不一致，会直接拒绝该请求。
func parseAdminUserID(headers http.Header) (string, error) {
	if headers == nil {
		return "", errInvalidAdminPrincipal
	}

	var userID string
	for _, headerName := range []string{captchaGatewayStandardUserIDHeader, captchaGatewayUserIDHeader} {
		value := strings.TrimSpace(headers.Get(headerName))
		if value == "" {
			continue
		}
		if userID == "" {
			userID = value
			continue
		}
		if value != userID {
			return "", errInvalidAdminPrincipal
		}
	}

	if userID == "" {
		return "", errInvalidAdminPrincipal
	}

	return userID, nil
}

// parseAdminRoles 会在兼容的新旧请求头别名中解析角色列表。
// 如果多个别名同时出现但角色集合不一致，会直接拒绝该请求。
func parseAdminRoles(headers http.Header) ([]int, error) {
	if headers == nil {
		return nil, errInvalidAdminPrincipal
	}

	var roles []int
	var hasRoles bool
	for _, headerName := range []string{captchaGatewayStandardUserRolesHeader, captchaGatewayUserRolesHeader} {
		// 角色可能出现在多个同名请求头里，所以这里读取全部值而不是只取一个。
		parsed, err := parseAdminRoleValues(headers.Values(headerName))
		if err != nil {
			return nil, errInvalidAdminPrincipal
		}
		if len(parsed) == 0 {
			continue
		}
		if !hasRoles {
			roles = parsed
			hasRoles = true
			continue
		}
		if !sameAdminRoleSet(roles, parsed) {
			return nil, errInvalidAdminPrincipal
		}
	}

	if !hasRoles {
		return nil, errInvalidAdminPrincipal
	}

	return roles, nil
}

func sameAdminRoleSet(left, right []int) bool {
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

// HasAnyRole 在身份包含任意一个给定角色时返回 true。
// 中间件会用它来判断调用方是否有权限访问目标接口。
func (p *AdminPrincipal) HasAnyRole(roles ...int) bool {
	if p == nil {
		// 没有身份信息时，不可能满足授权要求。
		return false
	}

	// 将当前角色列表转成集合，避免重复查找。
	current := make(map[int]struct{}, len(p.Roles))
	for _, role := range p.Roles {
		current[role] = struct{}{}
	}

	// 只要命中任意一个所需角色，就立即返回 true。
	for _, role := range roles {
		if _, ok := current[role]; ok {
			return true
		}
	}

	return false
}

// parseAdminRoleValues 会把原始的 X-User-Roles 请求头规范化为去重后的整数列表。
//
// 网关可能用不同格式输出角色，所以这里同时支持：
// - JSON 数组，例如 [1,2,3]
// - JSON 字符串数组，例如 ["1","2","3"]
// - 逗号分隔字符串，例如 1,2,3
func parseAdminRoleValues(values []string) ([]int, error) {
	// 去重的同时保持首次出现的顺序稳定。
	roles := make([]int, 0, len(values))
	seen := map[int]struct{}{}

	for _, raw := range values {
		// 空值或只有空白的头部值会被忽略。
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		// 根据值的外层形状选择解析器。
		var parsed []int
		var err error
		if strings.HasPrefix(trimmed, "[") {
			// 当头部值是结构化内容时，优先按 JSON 解析。
			parsed, err = parseAdminJSONRoles(trimmed)
		} else {
			// 对旧格式或代理拼接出来的头部，回退到逗号分隔解析。
			parsed, err = parseAdminDelimitedRoles(trimmed)
		}
		if err != nil {
			// 只要有一种表示方式不合法，就直接拒绝整个身份。
			return nil, err
		}

		// 在所有头部值之间去重，同时保留首次出现的顺序。
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

// parseAdminJSONRoles 支持两种 JSON 表达：数字数组或字符串数组。
// 先尝试数字数组，失败后再尝试字符串数组，以兼容不同网关实现。
func parseAdminJSONRoles(raw string) ([]int, error) {
	// 先尝试最严格、最紧凑的编码：[1,2,3]。
	var ints []int
	if err := json.Unmarshal([]byte(raw), &ints); err == nil {
		return ints, nil
	}

	// 如果网关把角色编码成字符串，这里将其统一转换成整数。
	var stringsValue []string
	if err := json.Unmarshal([]byte(raw), &stringsValue); err != nil {
		// 该值不是支持的任一种 JSON 格式。
		return nil, errInvalidAdminPrincipal
	}

	// 把每个字符串角色转成整数，只要有一个非法值就整体拒绝。
	roles := make([]int, 0, len(stringsValue))
	for _, value := range stringsValue {
		role, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			// 只要有一个角色格式错误，整条头部就不可用。
			return nil, errInvalidAdminPrincipal
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// parseAdminDelimitedRoles 解析旧式逗号分隔格式，例如 "1,2,3"。
// 转换前会去掉每一项两侧的空白和引号。
func parseAdminDelimitedRoles(raw string) ([]int, error) {
	// 先把头部值按逗号拆分，再逐项规范化。
	parts := strings.Split(raw, ",")
	roles := make([]int, 0, len(parts))
	for _, part := range parts {
		// 允许代理把值写成 "\"3\"" 或 " 3 " 这样的形式。
		trimmed := strings.TrimSpace(strings.Trim(part, "\""))
		if trimmed == "" {
			continue
		}

		// 每个角色都必须是合法的整数标识。
		role, err := strconv.Atoi(trimmed)
		if err != nil {
			// 只要有一个条目错误，这个头部就不能信任。
			return nil, errInvalidAdminPrincipal
		}
		roles = append(roles, role)
	}

	return roles, nil
}
