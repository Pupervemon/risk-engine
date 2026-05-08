package http

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func NewHealthRouter(redisClient *redis.Client, logger *zap.Logger, serviceInfo ServiceInfo, adminReader RiskAdminReader) *gin.Engine {
	return NewRiskRouter(redisClient, logger, serviceInfo, adminReader)
}
