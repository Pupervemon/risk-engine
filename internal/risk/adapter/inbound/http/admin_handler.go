package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	"github.com/gin-gonic/gin"
)

type RiskAdminReader interface {
	// ListRiskIPs 返回风险 IP 列表，支持分页和搜索条件。
	ListRiskIPs(ctx context.Context, query ports.RiskIPListQuery) (*ports.RiskIPListResponse, error)
	// GetRiskIP 返回单个 IP 的风险详情。
	GetRiskIP(ctx context.Context, ip string) (*domain.RiskIPDetail, error)
	// ListRiskIPEvents 返回某个 IP 关联的风险事件列表。
	ListRiskIPEvents(ctx context.Context, ip string, query ports.RiskIPEventsQuery) (*ports.RiskIPEventsResponse, error)
}

// RiskAdminHandler 提供风险管理后台使用的 HTTP 接口。
// 它只负责参数解析、错误转换和响应输出，具体业务逻辑交给 Reader 实现。
type RiskAdminHandler struct {
	Reader RiskAdminReader
}

// NewRiskAdminHandler 创建风险管理接口处理器。
func NewRiskAdminHandler(reader RiskAdminReader) *RiskAdminHandler {
	return &RiskAdminHandler{Reader: reader}
}

// ListRiskIPs 处理风险 IP 列表查询。
// 支持 limit、offset 和 q 三个查询参数，其中 q 用于模糊搜索。
func (h *RiskAdminHandler) ListRiskIPs(c *gin.Context) {
	// limit 和 offset 必须是非负整数；参数缺失时使用默认值 0。
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_LIMIT"})
		return
	}

	// 对 offset 做同样的输入校验，避免把非法字符串传给业务层。
	offset, ok := parseOptionalIntQuery(c, "offset")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_OFFSET"})
		return
	}

	// 将 HTTP 查询参数转换成业务层需要的查询对象。
	resp, err := h.Reader.ListRiskIPs(c.Request.Context(), ports.RiskIPListQuery{
		Limit:  limit,
		Offset: offset,
		Search: strings.TrimSpace(c.Query("q")),
	})
	if err != nil {
		// 业务层将 IP 格式错误统一收敛为 ErrInvalidRiskIP，这里转成 400。
		switch {
		case errors.Is(err, domain.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		default:
			// 其他错误视为服务端异常，由调用方感知为查询失败。
			c.JSON(http.StatusInternalServerError, gin.H{"error": "LIST_RISK_IPS_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetRiskIP 根据路径参数中的 IP 获取单条风险详情。
func (h *RiskAdminHandler) GetRiskIP(c *gin.Context) {
	// 路径参数由路由器负责提供，这里只做业务调用和错误映射。
	resp, err := h.Reader.GetRiskIP(c.Request.Context(), c.Param("ip"))
	if err != nil {
		// 非法 IP 直接返回 400，避免把无效请求继续向下传播。
		switch {
		case errors.Is(err, domain.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		// 业务层明确返回未找到时，对外暴露 404。
		case errors.Is(err, domain.ErrRiskIPNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "RISK_IP_NOT_FOUND"})
		default:
			// 未知错误统一按服务异常处理。
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GET_RISK_IP_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetRiskIPEvents 查询指定 IP 的风险事件列表。
// 接口参数和错误处理方式与 ListRiskIPs 保持一致，便于前端统一调用。
func (h *RiskAdminHandler) GetRiskIPEvents(c *gin.Context) {
	// 事件列表同样支持分页参数，且需要保证为非负整数。
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_LIMIT"})
		return
	}

	// offset 为可选参数，不传时默认从头开始。
	offset, ok := parseOptionalIntQuery(c, "offset")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_OFFSET"})
		return
	}

	// 将参数打包成业务层的事件查询对象。
	resp, err := h.Reader.ListRiskIPEvents(c.Request.Context(), c.Param("ip"), ports.RiskIPEventsQuery{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		// 与单 IP 查询保持一致，IP 不合法返回 400，不存在返回 404。
		switch {
		case errors.Is(err, domain.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		case errors.Is(err, domain.ErrRiskIPNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "RISK_IP_NOT_FOUND"})
		default:
			// 其余错误由服务端兜底处理。
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GET_RISK_IP_EVENTS_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

// parseOptionalIntQuery 解析可选的整数查询参数。
// 规则是：参数缺失时返回 0 和 true；参数存在但不是非负整数时返回 false。
func parseOptionalIntQuery(c *gin.Context, key string) (int, bool) {
	// 去掉首尾空白，避免 " 10 " 这种输入影响判断。
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, true
	}

	// 只接受非负整数，负数或非数字都视为非法输入。
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}
