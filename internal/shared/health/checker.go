package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// HealthStatus represents the overall health state.
type HealthStatus string

const (
	StatusUP   HealthStatus = "UP"
	StatusDOWN HealthStatus = "DOWN"
)

// HealthResponse is the standard health-check response payload.
type HealthResponse struct {
	Status     HealthStatus              `json:"status"`
	Components map[string]ComponentCheck `json:"components,omitempty"`
	Timestamp  string                    `json:"timestamp"`
}

// ComponentCheck is the status of a single dependency.
type ComponentCheck struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
}

// Checker validates shared dependencies such as Redis.
type Checker struct {
	redisClient *redis.Client
	logger      *zap.Logger
}

// NewChecker constructs a dependency health checker.
func NewChecker(redisClient *redis.Client, logger *zap.Logger) *Checker {
	return &Checker{
		redisClient: redisClient,
		logger:      logger,
	}
}

// CheckRedis verifies Redis connectivity.
func (c *Checker) CheckRedis(ctx context.Context) ComponentCheck {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := c.redisClient.Ping(ctx).Result(); err != nil {
		c.logger.Warn("redis health check failed", zap.Error(err))
		return ComponentCheck{
			Status:  StatusDOWN,
			Message: err.Error(),
		}
	}

	return ComponentCheck{
		Status:  StatusUP,
		Message: "redis connection healthy",
	}
}

// NewHealthRouter builds a generic health-check router.
func NewHealthRouter(redisClient *redis.Client, logger *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	checker := NewChecker(redisClient, logger)

	router.GET("/health", func(c *gin.Context) {
		ctx := c.Request.Context()

		response := HealthResponse{
			Status:     StatusUP,
			Components: make(map[string]ComponentCheck),
			Timestamp:  time.Now().Format(time.RFC3339),
		}

		redisCheck := checker.CheckRedis(ctx)
		response.Components["redis"] = redisCheck

		if redisCheck.Status == StatusDOWN {
			response.Status = StatusDOWN
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}

		c.JSON(http.StatusOK, response)
	})

	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if _, err := redisClient.Ping(ctx).Result(); err != nil {
			logger.Warn("readiness check failed", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	return router
}
