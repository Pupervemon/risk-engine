package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewCaptchaRouter builds the HTTP router for the captcha service.
func NewCaptchaRouter(handler *CaptchaHandler, healthHandler *HealthHandler) *gin.Engine {
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

		handler.Logger.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	})

	api := router.Group("/api/v1")
	{
		api.GET("/captcha", handler.GetCaptcha)
		api.POST("/captcha/verify", handler.VerifyCaptcha)
	}

	router.GET("/health", healthHandler.Health)
	router.GET("/actuator/health", healthHandler.DetailedHealth)
	router.GET("/actuator/health/liveness", healthHandler.Liveness)
	router.GET("/actuator/health/readiness", healthHandler.Readiness)

	return router
}
