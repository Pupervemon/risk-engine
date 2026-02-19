package transport

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewHealthRouter 创建健康检查路由 (Risk服务专用)
// 为了保持向后兼容，此函数委托给 NewRiskRouter
// 推荐直接使用 NewRiskRouter 以获得完整的路由配置
func NewHealthRouter(redisClient *redis.Client, logger *zap.Logger) *gin.Engine {
	return NewRiskRouter(redisClient, logger)
}
