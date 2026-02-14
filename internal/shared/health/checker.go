package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// HealthStatus 健康状态
type HealthStatus string

const (
	StatusUP   HealthStatus = "UP"
	StatusDOWN HealthStatus = "DOWN"
)

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status     HealthStatus              `json:"status"`
	Components map[string]ComponentCheck `json:"components,omitempty"`
	Timestamp  string                    `json:"timestamp"`
}

// ComponentCheck 组件检查结果
type ComponentCheck struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

// Checker 健康检查器
type Checker struct {
	redisClient *redis.Client
	logger      *zap.Logger
}

// NewChecker 创建健康检查器
func NewChecker(redisClient *redis.Client, logger *zap.Logger) *Checker {
	return &Checker{
		redisClient: redisClient,
		logger:      logger,
	}
}

// CheckRedis 检查Redis连接
func (c *Checker) CheckRedis(ctx context.Context) ComponentCheck {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := c.redisClient.Ping(ctx).Result(); err != nil {
		c.logger.Warn("Redis健康检查失败", zap.Error(err))
		return ComponentCheck{
			Status:  StatusDOWN,
			Message: err.Error(),
		}
	}

	return ComponentCheck{
		Status:  StatusUP,
		Message: "Redis连接正常",
	}
}

// NewHealthRouter 创建健康检查路由（通用版本）
func NewHealthRouter(redisClient *redis.Client, logger *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	checker := NewChecker(redisClient, logger)

	// 详细健康检查（检查所有依赖）
	router.GET("/health", func(c *gin.Context) {
		ctx := c.Request.Context()

		response := HealthResponse{
			Status:     StatusUP,
			Components: make(map[string]ComponentCheck),
			Timestamp:  time.Now().Format(time.RFC3339),
		}

		// 检查Redis
		redisCheck := checker.CheckRedis(ctx)
		response.Components["redis"] = redisCheck

		// 如果任何组件DOWN，则整体状态为DOWN
		if redisCheck.Status == StatusDOWN {
			response.Status = StatusDOWN
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		c.JSON(http.StatusOK, response)
	})

	// Kubernetes就绪探针（简化检查，仅检查Redis）
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if _, err := redisClient.Ping(ctx).Result(); err != nil {
			logger.Warn("就绪检查失败", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	// Kubernetes存活探针（快速检查，不检查依赖）
	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	return router
}
