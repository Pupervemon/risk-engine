package transport

import (
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewRiskRouter 创建风控服务的HTTP路由
// Risk服务主要通过gRPC提供业务接口，HTTP仅用于健康检查和监控
func NewRiskRouter(redisClient *redis.Client, logger *zap.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	// 请求日志中间件（过滤浏览器自动请求）
	router.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		// 跳过浏览器自动请求的日志
		ignorePaths := []string{
			"/favicon.ico",
			"/.well-known/appspecific/com.chrome.devtools.json",
		}
		for _, ignorePath := range ignorePaths {
			if path == ignorePath {
				return
			}
		}

		logger.Info("接收到请求",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	})

	// 创建健康检查处理器
	healthChecker := health.NewChecker(redisClient, logger)

	// 标准健康检查端点
	router.GET("/health", func(c *gin.Context) {
		ctx := c.Request.Context()

		response := health.HealthResponse{
			Status:     health.StatusUP,
			Components: make(map[string]health.ComponentCheck),
			Timestamp:  time.Now().Format(time.RFC3339),
		}

		// 检查Redis
		redisCheck := healthChecker.CheckRedis(ctx)
		response.Components["redis"] = redisCheck

		// 如果任何组件DOWN，则整体状态为DOWN
		if redisCheck.Status == health.StatusDOWN {
			response.Status = health.StatusDOWN
			c.JSON(503, response)
			return
		}

		c.JSON(200, response)
	})

	router.GET("/health/ready", func(c *gin.Context) {
		ctx := c.Request.Context()
		redisCheck := healthChecker.CheckRedis(ctx)

		if redisCheck.Status == health.StatusDOWN {
			logger.Warn("就绪检查失败", zap.String("error", redisCheck.Message))
			c.JSON(503, gin.H{
				"status": "DOWN",
				"error":  redisCheck.Message,
			})
			return
		}

		c.JSON(200, gin.H{"status": "UP"})
	})

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	// Actuator风格的健康检查端点（兼容Spring Boot监控工具）
	router.GET("/actuator/health", func(c *gin.Context) {
		ctx := c.Request.Context()

		response := health.HealthResponse{
			Status:     health.StatusUP,
			Components: make(map[string]health.ComponentCheck),
			Timestamp:  time.Now().Format(time.RFC3339),
		}

		redisCheck := healthChecker.CheckRedis(ctx)
		response.Components["redis"] = redisCheck

		if redisCheck.Status == health.StatusDOWN {
			response.Status = health.StatusDOWN
			c.JSON(503, response)
			return
		}

		c.JSON(200, response)
	})

	router.GET("/actuator/health/readiness", func(c *gin.Context) {
		ctx := c.Request.Context()
		redisCheck := healthChecker.CheckRedis(ctx)

		if redisCheck.Status == health.StatusDOWN {
			c.JSON(503, gin.H{"status": "DOWN"})
			return
		}

		c.JSON(200, gin.H{"status": "UP"})
	})

	router.GET("/actuator/health/liveness", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	// TODO: 未来可扩展的管理端点
	// admin := router.Group("/admin")
	// {
	//     // 黑名单管理接口（可选，当前通过gRPC提供）
	//     // admin.POST("/blacklist", handler.AddBlacklist)
	//     // admin.DELETE("/blacklist", handler.RemoveBlacklist)
	//
	//     // 频控规则管理接口（可选）
	//     // admin.GET("/rate-limit/rules", handler.GetRateLimitRules)
	//     // admin.PUT("/rate-limit/rules", handler.UpdateRateLimitRules)
	// }

	// 服务信息端点
	router.GET("/info", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service":     "risk-service",
			"version":     "1.0.0",
			"protocol":    "grpc",
			"description": "Risk Engine - 风控引擎服务",
			"endpoints": gin.H{
				"grpc":   "port 9090",
				"health": "/health, /health/ready, /health/live",
			},
		})
	})

	return router
}
