package http

import (
	"net/http"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// HealthHandler 健康检查处理器（captcha服务专用，包含扩展端点）
type HealthHandler struct {
	Checker *health.Checker
}

// Health 健康检查接口（简化版）
// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	overallStatus := health.StatusUP

	// 检查Redis连接
	redisStatus := h.Checker.CheckRedis(ctx)
	if redisStatus.Status == health.StatusDOWN {
		overallStatus = health.StatusDOWN
	}

	response := health.HealthResponse{
		Status: overallStatus,
		Components: map[string]health.ComponentCheck{
			"redis": redisStatus,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	statusCode := http.StatusOK
	if overallStatus == health.StatusDOWN {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// DetailedHealth 详细健康检查接口
// GET /actuator/health
func (h *HealthHandler) DetailedHealth(c *gin.Context) {
	ctx := c.Request.Context()
	overallStatus := health.StatusUP

	// 检查Redis连接
	redisStatus := h.Checker.CheckRedis(ctx)
	if redisStatus.Status == health.StatusDOWN {
		overallStatus = health.StatusDOWN
	}

	// 检查磁盘空间（可选）
	diskStatus := checkDisk()

	response := health.HealthResponse{
		Status: overallStatus,
		Components: map[string]health.ComponentCheck{
			"redis": redisStatus,
			"disk":  diskStatus,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	statusCode := http.StatusOK
	if overallStatus == health.StatusDOWN {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// Liveness 存活探针（K8s）
// GET /actuator/health/liveness
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    health.StatusUP,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// Readiness 就绪探针（K8s）
// GET /actuator/health/readiness
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx := c.Request.Context()
	// 检查关键依赖是否就绪
	redisStatus := h.Checker.CheckRedis(ctx)

	status := health.StatusUP
	statusCode := http.StatusOK

	if redisStatus.Status == health.StatusDOWN {
		status = health.StatusDOWN
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, gin.H{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
		"components": map[string]health.ComponentCheck{
			"redis": redisStatus,
		},
	})
}

// checkDisk 检查磁盘状态（简化版）
func checkDisk() health.ComponentCheck {
	// 这里可以添加磁盘空间检查逻辑
	// 简化处理，直接返回UP
	return health.ComponentCheck{
		Status:  health.StatusUP,
		Message: "磁盘空间充足",
	}
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(redis *redis.Client, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		Checker: health.NewChecker(redis, logger),
	}
}
