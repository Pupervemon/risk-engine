package transport

import (
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func NewRiskRouter(redisClient *redis.Client, logger *zap.Logger, serviceInfo ServiceInfo, adminReader RiskAdminReader) *gin.Engine {
	serviceInfo = serviceInfo.normalized()

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		ignorePaths := []string{
			"/favicon.ico",
			"/.well-known/appspecific/com.chrome.devtools.json",
		}
		for _, ignorePath := range ignorePaths {
			if path == ignorePath {
				return
			}
		}

		logger.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	})

	healthChecker := health.NewChecker(redisClient, logger)

	router.GET("/health", func(c *gin.Context) {
		ctx := c.Request.Context()
		response := health.HealthResponse{
			Status:     health.StatusUP,
			Components: map[string]health.ComponentCheck{},
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

	router.GET("/health/ready", func(c *gin.Context) {
		ctx := c.Request.Context()
		redisCheck := healthChecker.CheckRedis(ctx)
		if redisCheck.Status == health.StatusDOWN {
			logger.Warn("readiness check failed", zap.String("error", redisCheck.Message))
			c.JSON(503, gin.H{"status": "DOWN", "error": redisCheck.Message})
			return
		}

		c.JSON(200, gin.H{"status": "UP"})
	})

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP"})
	})

	router.GET("/actuator/health", func(c *gin.Context) {
		ctx := c.Request.Context()
		response := health.HealthResponse{
			Status:     health.StatusUP,
			Components: map[string]health.ComponentCheck{},
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

	if adminReader != nil {
		adminHandler := NewRiskAdminHandler(adminReader)
		admin := router.Group("/api/v1/admin")
		admin.Use(RiskAdminAuthMiddleware(logger, RoleTeacher, RoleAdmin))
		admin.GET("/risk-ips", adminHandler.ListRiskIPs)
		admin.GET("/risk-ips/:ip", adminHandler.GetRiskIP)
		admin.GET("/risk-ips/:ip/events", adminHandler.GetRiskIPEvents)
	}

	router.GET("/info", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service":     serviceInfo.Name,
			"version":     serviceInfo.Version,
			"protocol":    serviceInfo.Protocol,
			"description": serviceInfo.Description,
			"endpoints": gin.H{
				"http":         serviceInfo.httpEndpoint(),
				"grpc":         serviceInfo.grpcEndpoint(),
				"health":       "/health, /health/ready, /health/live",
				"admin_riskip": "/api/v1/admin/risk-ips, /api/v1/admin/risk-ips/{ip}, /api/v1/admin/risk-ips/{ip}/events",
			},
		})
	})

	return router
}
