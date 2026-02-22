package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewCaptchaRouter 创建验证码服务的HTTP路由
func NewCaptchaRouter(handler *CaptchaHandler, healthHandler *HealthHandler) *gin.Engine {
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

		handler.Logger.Info("接收到请求",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	})

	// API 路由组
	api := router.Group("/api/v1")
	{
		// 验证码相关接口
		api.GET("/captcha", handler.GetCaptcha)
		api.POST("/captcha/verify", handler.VerifyCaptcha)
	}

	// 健康检查路由
	router.GET("/health", healthHandler.Health)
	router.GET("/actuator/health", healthHandler.DetailedHealth)
	router.GET("/actuator/health/liveness", healthHandler.Liveness)
	router.GET("/actuator/health/readiness", healthHandler.Readiness)

	return router
}
