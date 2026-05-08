package http

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

	systemHandler := NewRiskSystemHandler(health.NewChecker(redisClient, logger), serviceInfo)

	router.GET("/health", systemHandler.Health)

	if adminReader != nil {
		adminHandler := NewRiskAdminHandler(adminReader)
		admin := router.Group("/api/v1/admin")
		admin.Use(RiskAdminAuthMiddleware(logger, RoleTeacher, RoleAdmin))
		admin.GET("/risk-ips", adminHandler.ListRiskIPs)
		admin.GET("/risk-ips/:ip", adminHandler.GetRiskIP)
		admin.GET("/risk-ips/:ip/events", adminHandler.GetRiskIPEvents)
	}

	router.GET("/info", systemHandler.Info)

	return router
}
