package bootstrap

import (
	grpcadapter "github.com/Pupervemon/risk-engine/internal/risk/adapter/inbound/grpc"
	httpadapter "github.com/Pupervemon/risk-engine/internal/risk/adapter/inbound/http"
	redisadapter "github.com/Pupervemon/risk-engine/internal/risk/adapter/outbound/redis"
	"github.com/Pupervemon/risk-engine/internal/risk/application"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RiskComponents struct {
	UseCase     *application.RiskUseCase
	GRPCService *grpcadapter.RiskControlService
	HTTPRouter  *gin.Engine
}

func NewRiskComponents(rdb *goredis.Client, cfg *config.RiskConfig, logger *zap.Logger) RiskComponents {
	if cfg == nil {
		cfg = &config.RiskConfig{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	options := RiskOptionsFromSharedConfig(&cfg.RiskRules)
	insights := redisadapter.NewRiskInsightRepository(rdb, cfg.RiskRules.Login.FailCountExpireMinutes, logger)
	useCase := application.NewRiskUseCase(
		redisadapter.NewBlacklistRepository(rdb),
		redisadapter.NewRateLimiter(rdb),
		redisadapter.NewLoginFailureRepository(rdb),
		insights,
		options,
		logger,
	)

	grpcService := grpcadapter.NewRiskControlService(useCase)
	httpRouter := httpadapter.NewRiskRouter(rdb, logger, httpadapter.ServiceInfo{
		Name:        cfg.Nacos.ServiceName,
		Version:     "1.0.0",
		Protocol:    "grpc",
		Description: "Risk Engine - risk control service",
		HTTPPort:    cfg.HTTP.Port,
		GRPCPort:    cfg.Grpc.Port,
	}, useCase)

	return RiskComponents{
		UseCase:     useCase,
		GRPCService: grpcService,
		HTTPRouter:  httpRouter,
	}
}
