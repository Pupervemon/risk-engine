package http

import "github.com/gin-gonic/gin"

func NewCaptchaRouter(handler *CaptchaHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api/v1")
	api.GET("/captcha", handler.GetCaptcha)
	api.POST("/captcha/verify", handler.VerifyCaptcha)

	return router
}
