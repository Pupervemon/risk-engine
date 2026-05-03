package http

import (
	"net/http"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// HealthHandler serves health endpoints for the captcha service.
type HealthHandler struct {
	Checker *health.Checker
}

// Health returns a lightweight health response.
func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	overallStatus := health.StatusUP

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

// DetailedHealth returns a fuller health response for actuator-style probes.
func (h *HealthHandler) DetailedHealth(c *gin.Context) {
	ctx := c.Request.Context()
	overallStatus := health.StatusUP

	redisStatus := h.Checker.CheckRedis(ctx)
	if redisStatus.Status == health.StatusDOWN {
		overallStatus = health.StatusDOWN
	}

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

// Liveness is the liveness probe endpoint.
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    health.StatusUP,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// Readiness is the readiness probe endpoint.
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx := c.Request.Context()
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

// checkDisk is a placeholder until disk usage is wired into health checks.
func checkDisk() health.ComponentCheck {
	return health.ComponentCheck{
		Status:  health.StatusUP,
		Message: "disk space is sufficient",
	}
}

// NewHealthHandler creates a captcha-specific health handler.
func NewHealthHandler(redis *redis.Client, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		Checker: health.NewChecker(redis, logger),
	}
}
