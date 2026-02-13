package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewCaptchaRouter(handler *CaptchaHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	// 请求日志中间件
	router.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		handler.Logger.Info("接收到请求",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	})

	// 完善的 CORS 中间件
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	api := router.Group("/api/v1")
	api.GET("/captcha", handler.GetCaptcha)
	api.POST("/captcha/verify", handler.VerifyCaptcha)

	return router
}
