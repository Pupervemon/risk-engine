package transport

import (
	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewHealthRouter 创建健康检查路由 (Risk服务专用)
// Risk服务的健康检查功能委托给shared/health模块
func NewHealthRouter(redisClient *redis.Client, logger *zap.Logger) *gin.Engine {
	return health.NewHealthRouter(redisClient, logger)
}
